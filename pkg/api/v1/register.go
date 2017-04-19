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

package v1

import (
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupName is the group name use in this package
const GroupName = ""

// SchemeGroupVersion is group version used to register these objects
var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1"}

// Resource takes an unqualified resource and returns a Group qualified GroupResource
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes, v1.AddDefaultingFuncs, addConversionFuncs, addFastPathConversionFuncs)
	AddToScheme   = SchemeBuilder.AddToScheme
)

// Adds the list of known types to api.Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&v1.Pod{},
		&v1.PodList{},
		&v1.PodStatusResult{},
		&v1.PodTemplate{},
		&v1.PodTemplateList{},
		&v1.ReplicationController{},
		&v1.ReplicationControllerList{},
		&v1.Service{},
		&v1.ServiceProxyOptions{},
		&v1.ServiceList{},
		&v1.Endpoints{},
		&v1.EndpointsList{},
		&v1.Node{},
		&v1.NodeList{},
		&v1.NodeProxyOptions{},
		&v1.Binding{},
		&v1.Event{},
		&v1.EventList{},
		&v1.List{},
		&v1.LimitRange{},
		&v1.LimitRangeList{},
		&v1.ResourceQuota{},
		&v1.ResourceQuotaList{},
		&v1.Namespace{},
		&v1.NamespaceList{},
		&v1.Secret{},
		&v1.SecretList{},
		&v1.ServiceAccount{},
		&v1.ServiceAccountList{},
		&v1.PersistentVolume{},
		&v1.PersistentVolumeList{},
		&v1.PersistentVolumeClaim{},
		&v1.PersistentVolumeClaimList{},
		&v1.PodAttachOptions{},
		&v1.PodLogOptions{},
		&v1.PodExecOptions{},
		&v1.PodPortForwardOptions{},
		&v1.PodProxyOptions{},
		&v1.ComponentStatus{},
		&v1.ComponentStatusList{},
		&v1.SerializedReference{},
		&v1.RangeAllocation{},
		&v1.ConfigMap{},
		&v1.ConfigMapList{},
	)

	// Add common types
	scheme.AddKnownTypes(SchemeGroupVersion, &metav1.Status{})

	// Add the watch version that applies
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
