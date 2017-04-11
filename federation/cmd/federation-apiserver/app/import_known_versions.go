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

package app

// These imports are the API groups the client will support.
import (
	"k8s.io/kubernetes/federation/apis/core"
	coreinstall "k8s.io/kubernetes/federation/apis/core/install"
	federationinstall "k8s.io/kubernetes/federation/apis/federation/install"
	"k8s.io/kubernetes/pkg/api/scheme"
	autoscalinginstall "k8s.io/kubernetes/pkg/apis/autoscaling/install"
	batchinstall "k8s.io/kubernetes/pkg/apis/batch/install"
	extensionsinstall "k8s.io/kubernetes/pkg/apis/extensions/install"
)

func init() {
	coreinstall.Install(core.GroupFactoryRegistry, core.Registry, core.Scheme)
	federationinstall.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	extensionsinstall.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	autoscalinginstall.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
	batchinstall.Install(scheme.GroupFactoryRegistry, scheme.Registry, scheme.Scheme)
}
