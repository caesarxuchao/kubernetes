/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/typed/dynamic"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/util/sets"
)

func TestNewGarbageCollector(t *testing.T) {
	clientPool := dynamic.NewClientPool(&restclient.Config{}, dynamic.LegacyAPIPathResolverFunc)
	podResource := []unversioned.GroupVersionResource{{Version: "v1", Resource: "pods"}}
	gc, err := NewGarbageCollector(clientPool, podResource)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 1, len(gc.monitors))
}

func newDanglingPod() *v1.Pod {
	return &v1.Pod{
		TypeMeta: unversioned.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      "ToBeDeletedPod",
			Namespace: "ns1",
			OwnerReferences: []v1.OwnerReference{
				{
					Kind:       "ReplicationController",
					Name:       "owner1",
					UID:        "123",
					APIVersion: "v1",
				},
			},
		},
	}
}

func TestCheckGarbage(t *testing.T) {
	testHandler := &fakeActionHandler{statusCode: map[string]int{
		"GET" + "/api/v1/namespaces/ns1/replicationcontrollers/owner1": 404,
	}}
	podResource := []unversioned.GroupVersionResource{{Version: "v1", Resource: "pods"}}
	srv, clientConfig := testServerAndClientConfig(testHandler.ServeHTTP)
	defer srv.Close()
	clientPool := dynamic.NewClientPool(clientConfig, dynamic.LegacyAPIPathResolverFunc)
	gc, err := NewGarbageCollector(clientPool, podResource)
	if err != nil {
		t.Fatal(err)
	}
	store := gc.monitors[0].Store
	pod := newDanglingPod()
	store.Add(pod)
	storeKey := itemRef{store, getKey(pod, t)}
	err = gc.processItem(storeKey)
	if err != nil {
		t.Errorf("Unexpected Error: %v", err)
	}
	expectedActionSet := sets.NewString()
	expectedActionSet.Insert("GET=/api/v1/namespaces/ns1/replicationcontrollers/owner1")
	expectedActionSet.Insert("DELETE=/api/v1/namespaces/ns1/pods/ToBeDeletedPod")
	actualActionSet := sets.NewString()
	for _, action := range testHandler.actions {
		actualActionSet.Insert(action.String())
	}
	if !expectedActionSet.Equal(actualActionSet) {
		t.Errorf("expected actions:\n%v\n but got:\n%v\nDifference:\n%v", expectedActionSet,
			actualActionSet, expectedActionSet.Difference(actualActionSet))
	}
}

// TODO: this code is shared with the deployment_controller_test. Move it to a common place.
func getKey(obj interface{}, t *testing.T) string {
	if key, err := controller.KeyFunc(obj); err != nil {
		t.Errorf("Unexpected error getting key: %v", err)
		return ""
	} else {
		return key
	}
}

//TODO: this test code is shared with the namespace_controller_test. Move it to pkg/util/testing/fake_handler.go
// fakeAction records information about requests to aid in testing.
type fakeAction struct {
	method string
	path   string
}

// String returns method=path to aid in testing
func (f *fakeAction) String() string {
	return strings.Join([]string{f.method, f.path}, "=")
}

// fakeActionHandler holds a list of fakeActions received
type fakeActionHandler struct {
	// statusCode returned by this handler for different method + path.
	statusCode map[string]int

	lock    sync.Mutex
	actions []fakeAction
}

// ServeHTTP logs the action that occurred and always returns the associated status code
func (f *fakeActionHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	f.lock.Lock()
	defer f.lock.Unlock()

	f.actions = append(f.actions, fakeAction{method: request.Method, path: request.URL.Path})
	code, ok := f.statusCode[request.Method+request.URL.Path]
	if !ok {
		code = 200
	}
	response.WriteHeader(code)
	response.Write([]byte("{\"kind\": \"List\"}"))
}

// testServerAndClientConfig returns a server that listens and a config that can reference it
func testServerAndClientConfig(handler func(http.ResponseWriter, *http.Request)) (*httptest.Server, *restclient.Config) {
	srv := httptest.NewServer(http.HandlerFunc(handler))
	config := &restclient.Config{
		Host: srv.URL,
	}
	return srv, config
}
