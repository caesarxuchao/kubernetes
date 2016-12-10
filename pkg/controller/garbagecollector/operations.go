/*
Copyright 2016 The Kubernetes Authors.

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

package garbagecollector

import (
	"fmt"

	"github.com/golang/glog"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/runtime/schema"
)

// apiResource consults the REST mapper to translate an <apiVersion, kind,
// namespace> tuple to a unversioned.APIResource struct.
func (gc *GarbageCollector) apiResource(apiVersion, kind string, namespaced bool) (*unversioned.APIResource, error) {
	fqKind := schema.FromAPIVersionAndKind(apiVersion, kind)
	mapping, err := gc.restMapper.RESTMapping(fqKind.GroupKind(), apiVersion)
	if err != nil {
		return nil, fmt.Errorf("unable to get REST mapping for kind: %s, version: %s", kind, apiVersion)
	}
	glog.V(6).Infof("map kind %s, version %s to resource %s", kind, apiVersion, mapping.Resource)
	resource := unversioned.APIResource{
		Name:       mapping.Resource,
		Namespaced: namespaced,
		Kind:       kind,
	}
	return &resource, nil
}

func (gc *GarbageCollector) deleteObject(item objectReference, policy v1.DeletePropagationPolicy) error {
	fqKind := schema.FromAPIVersionAndKind(item.APIVersion, item.Kind)
	client, err := gc.clientPool.ClientForGroupVersionKind(fqKind)
	gc.registeredRateLimiter.registerIfNotPresent(fqKind.GroupVersion(), client, "garbage_collector_operation")
	resource, err := gc.apiResource(item.APIVersion, item.Kind, len(item.Namespace) != 0)
	if err != nil {
		return err
	}
	uid := item.UID
	preconditions := v1.Preconditions{UID: &uid}
	deleteOptions := v1.DeleteOptions{Preconditions: &preconditions, PropagationPolicy: &policy}
	return client.Resource(resource, item.Namespace).Delete(item.Name, &deleteOptions)
}

func (gc *GarbageCollector) getObject(item objectReference) (*runtime.Unstructured, error) {
	fqKind := schema.FromAPIVersionAndKind(item.APIVersion, item.Kind)
	client, err := gc.clientPool.ClientForGroupVersionKind(fqKind)
	gc.registeredRateLimiter.registerIfNotPresent(fqKind.GroupVersion(), client, "garbage_collector_operation")
	resource, err := gc.apiResource(item.APIVersion, item.Kind, len(item.Namespace) != 0)
	if err != nil {
		return nil, err
	}
	return client.Resource(resource, item.Namespace).Get(item.Name)
}

func (gc *GarbageCollector) updateObject(item objectReference, obj *runtime.Unstructured) (*runtime.Unstructured, error) {
	fqKind := schema.FromAPIVersionAndKind(item.APIVersion, item.Kind)
	client, err := gc.clientPool.ClientForGroupVersionKind(fqKind)
	gc.registeredRateLimiter.registerIfNotPresent(fqKind.GroupVersion(), client, "garbage_collector_operation")
	resource, err := gc.apiResource(item.APIVersion, item.Kind, len(item.Namespace) != 0)
	if err != nil {
		return nil, err
	}
	return client.Resource(resource, item.Namespace).Update(obj)
}

func (gc *GarbageCollector) patchObject(item objectReference, patch []byte) (*runtime.Unstructured, error) {
	fqKind := schema.FromAPIVersionAndKind(item.APIVersion, item.Kind)
	client, err := gc.clientPool.ClientForGroupVersionKind(fqKind)
	gc.registeredRateLimiter.registerIfNotPresent(fqKind.GroupVersion(), client, "garbage_collector_operation")
	resource, err := gc.apiResource(item.APIVersion, item.Kind, len(item.Namespace) != 0)
	if err != nil {
		return nil, err
	}
	return client.Resource(resource, item.Namespace).Patch(item.Name, api.StrategicMergePatchType, patch)
}
