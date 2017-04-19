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

package testapi

import (
	federationinstall "k8s.io/kubernetes/federation/apis/federation/install"
	"k8s.io/kubernetes/pkg/api"
	coreinstall "k8s.io/kubernetes/pkg/api/install"
	appsinstall "k8s.io/kubernetes/pkg/apis/apps/install"
	authenticationinstall "k8s.io/kubernetes/pkg/apis/authentication/install"
	authorizationinstall "k8s.io/kubernetes/pkg/apis/authorization/install"
	autoscalinginstall "k8s.io/kubernetes/pkg/apis/autoscaling/install"
	batchinstall "k8s.io/kubernetes/pkg/apis/batch/install"
	certificatesinstall "k8s.io/kubernetes/pkg/apis/certificates/install"
	componentconfiginstall "k8s.io/kubernetes/pkg/apis/componentconfig/install"
	extensionsinstall "k8s.io/kubernetes/pkg/apis/extensions/install"
	imagepolicyinstall "k8s.io/kubernetes/pkg/apis/imagepolicy/install"
	policyinstall "k8s.io/kubernetes/pkg/apis/policy/install"
	rbacinstall "k8s.io/kubernetes/pkg/apis/rbac/install"
	settingsinstall "k8s.io/kubernetes/pkg/apis/settings/install"
	storageinstall "k8s.io/kubernetes/pkg/apis/storage/install"
)

// WARNING: avoid using this function when possible. No tests should rely on the global api.api.Registry, api.GroupFactoryRegistry, or api.api.Scheme
func InstallGlobally() {
	coreinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	federationinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	appsinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	authenticationinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	authorizationinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	autoscalinginstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	batchinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	certificatesinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	componentconfiginstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	extensionsinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	imagepolicyinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	policyinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	rbacinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	settingsinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
	storageinstall.Install(api.GroupFactoryRegistry, api.Registry, api.Scheme)
}
