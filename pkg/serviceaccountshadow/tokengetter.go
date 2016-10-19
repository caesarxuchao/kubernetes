/*
Copyright 2014 The Kubernetes Authors.

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

package serviceaccount

import (
	clientset "k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"
)

// clientGetter implements ServiceAccountTokenGetter using a clientset.Interface
type clientGetter struct {
	client clientset.Interface
}

// NewGetterFromClient returns a ServiceAccountTokenGetter that
// uses the specified client to retrieve service accounts and secrets.
// The client should NOT authenticate using a service account token
// the returned getter will be used to retrieve, or recursion will result.
func NewGetterFromClient(c clientset.Interface) ServiceAccountTokenGetter {
	return clientGetter{c}
}
func (c clientGetter) GetServiceAccount(namespace, name string) (*v1.ServiceAccount, error) {
	return c.client.Core().ServiceAccounts(namespace).Get(name)
}
func (c clientGetter) GetSecret(namespace, name string) (*v1.Secret, error) {
	return c.client.Core().Secrets(namespace).Get(name)
}
