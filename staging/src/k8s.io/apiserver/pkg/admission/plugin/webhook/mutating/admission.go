/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package webhook delegates admission checks to dynamically configured webhooks.
package mutating

import (
	"context"
	"fmt"
	"io"
	"path"

	"github.com/golang/glog"

	admissionv1alpha1 "k8s.io/api/admission/v1alpha1"
	"k8s.io/api/admissionregistration/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apiserver/pkg/admission"
	"k8s.io/apiserver/pkg/admission/configuration"
	genericadmissioninit "k8s.io/apiserver/pkg/admission/initializer"
	"k8s.io/apiserver/pkg/admission/plugin/webhook/config"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// Name of admission plug-in
	PluginName = "MutatingAdmissionWebhook"
)

type ErrCallingWebhook struct {
	WebhookName string
	Reason      error
}

func (e *ErrCallingWebhook) Error() string {
	if e.Reason != nil {
		return fmt.Sprintf("failed calling admission webhook %q: %v", e.WebhookName, e.Reason)
	}
	return fmt.Sprintf("failed calling admission webhook %q; no further details available", e.WebhookName)
}

// Register registers a plugin
func Register(plugins *admission.Plugins) {
	plugins.Register(PluginName, func(configFile io.Reader) (admission.Interface, error) {
		plugin, err := NewGenericAdmissionWebhook(configFile)
		if err != nil {
			return nil, err
		}

		return plugin, nil
	})
}

// WebhookSource can list dynamic webhook plugins.
type WebhookSource interface {
	Run(stopCh <-chan struct{})
	ExternalAdmissionHooks() (*v1alpha1.ExternalAdmissionHookConfiguration, error)
}

// NewGenericAdmissionWebhook returns a generic admission webhook plugin.
func NewGenericAdmissionWebhook(configFile io.Reader) (*GenericAdmissionWebhook, error) {
	kubeconfigFile := ""
	if configFile != nil {
		// TODO: move this to a versioned configuration file format
		var config AdmissionConfig
		d := yaml.NewYAMLOrJSONDecoder(configFile, 4096)
		err := d.Decode(&config)
		if err != nil {
			return nil, err
		}
		kubeconfigFile = config.KubeConfigFile
	}
	authInfoResolver, err := config.NewDefaultAuthenticationInfoResolver(kubeconfigFile)
	if err != nil {
		return nil, err
	}

	return &GenericAdmissionWebhook{
		Handler: admission.NewHandler(
			admission.Connect,
			admission.Create,
			admission.Delete,
			admission.Update,
		),
		authInfoResolver: authInfoResolver,
		serviceResolver:  defaultServiceResolver{},
	}, nil
}

// GenericAdmissionWebhook is an implementation of admission.Interface.
type GenericAdmissionWebhook struct {
	*admission.Handler
	hookSource      WebhookSource
	serviceResolver config.ServiceResolver
	// TODO: only keep one of two of the three
	negotiatedSerializer runtime.NegotiatedSerializer
	convertor            runtime.ObjectConvertor
	scheme               *runtime.Scheme

	authInfoResolver config.AuthenticationInfoResolver
}

var (
	_ = genericadmissioninit.WantsExternalKubeClientSet(&GenericAdmissionWebhook{})
)

// TODO find a better way wire this, but keep this pull small for now.
func (a *GenericAdmissionWebhook) SetAuthenticationInfoResolverWrapper(wrapper config.AuthenticationInfoResolverWrapper) {
	if wrapper != nil {
		a.authInfoResolver = wrapper(a.authInfoResolver)
	}
}

// SetServiceResolver sets a service resolver for the webhook admission plugin.
// Passing a nil resolver does not have an effect, instead a default one will be used.
func (a *GenericAdmissionWebhook) SetServiceResolver(sr config.ServiceResolver) {
	if sr != nil {
		a.serviceResolver = sr
	}
}

// SetScheme sets a serializer(NegotiatedSerializer) which is derived from the scheme
func (a *GenericAdmissionWebhook) SetScheme(scheme *runtime.Scheme) {
	if scheme != nil {
		a.negotiatedSerializer = serializer.NegotiatedSerializerWrapper(runtime.SerializerInfo{
			Serializer: serializer.NewCodecFactory(scheme).LegacyCodec(admissionv1alpha1.SchemeGroupVersion),
		})
		a.convertor = scheme
		a.scheme = scheme
	}
}

// WantsExternalKubeClientSet defines a function which sets external ClientSet for admission plugins that need it
func (a *GenericAdmissionWebhook) SetExternalKubeClientSet(client clientset.Interface) {
	a.hookSource = configuration.NewExternalAdmissionHookConfigurationManager(client.Admissionregistration().ExternalAdmissionHookConfigurations())
}

// Validator holds Validate functions, which are responsible for validation of initialized shared resources
// and should be implemented on admission plugins
func (a *GenericAdmissionWebhook) Validate() error {
	if a.hookSource == nil {
		return fmt.Errorf("the GenericAdmissionWebhook admission plugin requires a Kubernetes client to be provided")
	}
	if a.negotiatedSerializer == nil {
		return fmt.Errorf("the GenericAdmissionWebhook admission plugin requires a runtime.Scheme to be provided to derive a serializer")
	}
	go a.hookSource.Run(wait.NeverStop)
	return nil
}

func (a *GenericAdmissionWebhook) loadConfiguration(attr admission.Attributes) (*v1alpha1.ExternalAdmissionHookConfiguration, error) {
	hookConfig, err := a.hookSource.ExternalAdmissionHooks()
	// if ExternalAdmissionHook configuration is disabled, fail open
	if err == configuration.ErrDisabled {
		return &v1alpha1.ExternalAdmissionHookConfiguration{}, nil
	}
	if err != nil {
		e := apierrors.NewServerTimeout(attr.GetResource().GroupResource(), string(attr.GetOperation()), 1)
		e.ErrStatus.Message = fmt.Sprintf("Unable to refresh the ExternalAdmissionHook configuration: %v", err)
		e.ErrStatus.Reason = "LoadingConfiguration"
		e.ErrStatus.Details.Causes = append(e.ErrStatus.Details.Causes, metav1.StatusCause{
			Type:    "ExternalAdmissionHookConfigurationFailure",
			Message: "An error has occurred while refreshing the externalAdmissionHook configuration, no resources can be created/updated/deleted/connected until a refresh succeeds.",
		})
		return nil, e
	}
	return hookConfig, nil
}

// TODO: update this struct when we get the AdmissionResponse type fixed.
type intermidiateAdmissionResult struct {
	versionedObject    runtime.Object
	versionedOldObject runtime.Object
	mutatedObjRaw      []byte
}

// Admit makes an admission decision based on the request attributes.
func (a *GenericAdmissionWebhook) Admit(attr admission.Attributes) error {
	hookConfig, err := a.loadConfiguration(attr)
	if err != nil {
		return err
	}
	hooks := hookConfig.ExternalAdmissionHooks
	ctx := context.TODO()

	ir := &intermidiateAdmissionResult{}
	for _, hook := range hooks {
		err = a.callHook(ctx, &hook, attr, ir)
		if err == nil {
			continue
		}
		ignoreClientCallFailures := hook.FailurePolicy != nil && *hook.FailurePolicy == v1alpha1.Ignore
		if callErr, ok := err.(*ErrCallingWebhook); ok {
			if ignoreClientCallFailures {
				glog.Warningf("Failed calling webhook, failing open %v: %v", hook.Name, callErr)
				utilruntime.HandleError(callErr)
				// Since we are failing open to begin with, we do not send an error down the channel
				continue
			}
			glog.Warningf("Failed calling webhook, failing closed %v: %v", hook.Name, err)
			return err
		}
		glog.Warningf("rejected by webhook %v %t: %v", hook.Name, err, err)
		return err
	}
	if len(ir.mutatedObjRaw) == 0 {
		// no webhook is called
		return nil
	}
	// TODO: don't construct a codec every time.
	// jsonSerializer := runtimejson.NewSerializer(runtimejson.DefaultMetaFactory, a.scheme, a.scheme, false)
	// gvk := attr.GetKind()
	// _, _, err = jsonSerializer.Decode(ir.mutatedObjRaw, &gvk, attr.GetObject())
	fmt.Printf("CHAO: final mutatedObjRaw=%s\n", ir.mutatedObjRaw)
	codec := serializer.NewCodecFactory(a.scheme).LegacyCodec()
	_, _, err = codec.Decode(ir.mutatedObjRaw, nil, attr.GetObject())
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	fmt.Printf("CHAO: final attr.GetObject()=%#v\n", attr.GetObject())
	return nil
}

func (a *GenericAdmissionWebhook) callHook(ctx context.Context, h *v1alpha1.ExternalAdmissionHook, attr admission.Attributes, ir *intermidiateAdmissionResult) error {
	matches := false
	for _, r := range h.Rules {
		m := RuleMatcher{Rule: r, Attr: attr}
		if m.Matches() {
			matches = true
			break
		}
	}
	if !matches {
		return nil
	}

	fmt.Println("CHAO: going create admission review")

	// Make the webhook request
	request, err := createAdmissionReview(a.convertor, attr, ir)
	if err != nil {
		return apierrors.NewInternalError(err)
	}
	client, err := a.hookClient(h)
	if err != nil {
		return &ErrCallingWebhook{WebhookName: h.Name, Reason: err}
	}
	response := &admissionv1alpha1.AdmissionReview{}
	if err := client.Post().Context(ctx).Body(&request).Do().Into(response); err != nil {
		return &ErrCallingWebhook{WebhookName: h.Name, Reason: err}
	}

	if response.Status.Allowed {
		// TODO: use the new field once we have that
		fmt.Printf("CHAO: setting the mutatedObjRaw to %s\n", response.Spec.Object.Raw)
		ir.mutatedObjRaw = response.Spec.Object.Raw
		return nil
	}

	// TODO: check if this is needed when we have the new api
	if response.Status.Result == nil {
		return fmt.Errorf("admission webhook %q denied the request without explanation", h.Name)
	}

	return &apierrors.StatusError{
		ErrStatus: *response.Status.Result,
	}
}

func (a *GenericAdmissionWebhook) hookClient(h *v1alpha1.ExternalAdmissionHook) (*rest.RESTClient, error) {
	serverName := h.ClientConfig.Service.Name + "." + h.ClientConfig.Service.Namespace + ".svc"
	u, err := a.serviceResolver.ResolveEndpoint(h.ClientConfig.Service.Namespace, h.ClientConfig.Service.Name)
	if err != nil {
		return nil, err
	}

	// TODO: cache these instead of constructing one each time
	restConfig, err := a.authInfoResolver.ClientConfigFor(serverName)
	if err != nil {
		return nil, err
	}
	cfg := rest.CopyConfig(restConfig)
	cfg.Host = u.Host
	cfg.APIPath = path.Join(u.Path, h.ClientConfig.URLPath)
	cfg.TLSClientConfig.ServerName = serverName
	cfg.TLSClientConfig.CAData = h.ClientConfig.CABundle
	cfg.ContentConfig.NegotiatedSerializer = a.negotiatedSerializer
	cfg.ContentConfig.ContentType = runtime.ContentTypeJSON
	return rest.UnversionedRESTClientFor(cfg)
}
