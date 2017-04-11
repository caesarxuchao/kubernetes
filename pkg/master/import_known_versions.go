/*
Copyright 2015 The Kubernetes Authors.

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

package master

// These imports are the API groups the API server will support.
import (
	"fmt"

	core "k8s.io/kubernetes/pkg/api/install"
	"k8s.io/kubernetes/pkg/api/scheme"
	apps "k8s.io/kubernetes/pkg/apis/apps/install"
	authentication "k8s.io/kubernetes/pkg/apis/authentication/install"
	authorization "k8s.io/kubernetes/pkg/apis/authorization/install"
	autoscaling "k8s.io/kubernetes/pkg/apis/autoscaling/install"
	batch "k8s.io/kubernetes/pkg/apis/batch/install"
	certificates "k8s.io/kubernetes/pkg/apis/certificates/install"
	componentconfig "k8s.io/kubernetes/pkg/apis/componentconfig/install"
	extensions "k8s.io/kubernetes/pkg/apis/extensions/install"
	imagepolicy "k8s.io/kubernetes/pkg/apis/imagepolicy/install"
	policy "k8s.io/kubernetes/pkg/apis/policy/install"
	rbac "k8s.io/kubernetes/pkg/apis/rbac/install"
	settings "k8s.io/kubernetes/pkg/apis/settings/install"
	storage "k8s.io/kubernetes/pkg/apis/storage/install"
)

func init() {
	core.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	apps.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	authentication.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	authorization.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	autoscaling.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	batch.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	certificates.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	componentconfig.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	extensions.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	imagepolicy.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	policy.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	rbac.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	settings.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	storage.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)

	if missingVersions := scheme.Registry.ValidateEnvRequestedVersions(); len(missingVersions) != 0 {
		panic(fmt.Sprintf("KUBE_API_VERSIONS contains versions that are not installed: %q.", missingVersions))
	}
}
