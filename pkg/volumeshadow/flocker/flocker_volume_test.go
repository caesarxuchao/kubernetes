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

package flocker

import (
	"fmt"
	"testing"

	"k8s.io/client-go/1.5/pkg/api/resource"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/kubernetes/pkg/api/unversioned"
	utiltesting "k8s.io/kubernetes/pkg/util/testing"
	"k8s.io/kubernetes/pkg/volumeshadow"
	volumetest "k8s.io/kubernetes/pkg/volumeshadow/testing"

	"github.com/stretchr/testify/assert"
)

func newTestableProvisioner(assert *assert.Assertions, options volumeshadow.VolumeOptions) volumeshadow.Provisioner {
	tmpDir, err := utiltesting.MkTmpdir("flockervolumeTest")
	assert.NoError(err, fmt.Sprintf("can't make a temp dir: %v", err))

	plugMgr := volumeshadow.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volumetest.NewFakeVolumeHost(tmpDir, nil, nil, "" /* rootContext */))

	plug, err := plugMgr.FindPluginByName(pluginName)
	assert.NoError(err, "Can't find the plugin by name")

	provisioner, err := plug.(*flockerPlugin).newProvisionerInternal(options, &fakeFlockerUtil{})

	return provisioner
}

func TestProvision(t *testing.T) {
	assert := assert.New(t)

	cap := resource.MustParse("3Gi")
	options := volumeshadow.VolumeOptions{
		Capacity: cap,
		AccessModes: []v1.PersistentVolumeAccessMode{
			v1.ReadWriteOnce,
		},
		PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
	}

	provisioner := newTestableProvisioner(assert, options)

	persistentSpec, err := provisioner.Provision()
	assert.NoError(err, "Provision() failed: ", err)

	cap = persistentSpec.Spec.Capacity[v1.ResourceStorage]

	assert.Equal(int64(3*1024*1024*1024), cap.Value())

	assert.Equal(
		"test-flocker-volume-uuid",
		persistentSpec.Spec.PersistentVolumeSource.Flocker.DatasetUUID,
	)

	assert.Equal(
		map[string]string{"fakeflockerutil": "yes"},
		persistentSpec.Labels,
	)

	// parameters are not supported
	options = volumeshadow.VolumeOptions{
		Capacity: cap,
		AccessModes: []v1.PersistentVolumeAccessMode{
			v1.ReadWriteOnce,
		},
		PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
		Parameters: map[string]string{
			"not-supported-params": "test123",
		},
	}

	provisioner = newTestableProvisioner(assert, options)
	persistentSpec, err = provisioner.Provision()
	assert.Error(err, "Provision() did not fail with Parameters specified")

	// selectors are not supported
	options = volumeshadow.VolumeOptions{
		Capacity: cap,
		AccessModes: []v1.PersistentVolumeAccessMode{
			v1.ReadWriteOnce,
		},
		PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
		Selector:                      &unversioned.LabelSelector{MatchLabels: map[string]string{"key": "value"}},
	}

	provisioner = newTestableProvisioner(assert, options)
	persistentSpec, err = provisioner.Provision()
	assert.Error(err, "Provision() did not fail with Selector specified")

}
