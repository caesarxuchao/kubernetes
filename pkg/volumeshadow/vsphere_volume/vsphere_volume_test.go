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

package vsphere_volume

import (
	"fmt"
	"os"
	"path"
	"testing"

	"k8s.io/client-go/1.5/pkg/api/resource"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/types"
	"k8s.io/kubernetes/pkg/util/mount"
	utiltesting "k8s.io/kubernetes/pkg/util/testing"
	"k8s.io/kubernetes/pkg/volumeshadow"
	volumetest "k8s.io/kubernetes/pkg/volumeshadow/testing"
)

func TestCanSupport(t *testing.T) {
	tmpDir, err := utiltesting.MkTmpdir("vsphereVolumeTest")
	if err != nil {
		t.Fatalf("can't make a temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	plugMgr := volumeshadow.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volumetest.NewFakeVolumeHost(tmpDir, nil, nil, "" /* rootContext */))

	plug, err := plugMgr.FindPluginByName("kubernetes.io/vsphere-volume")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}
	if plug.GetPluginName() != "kubernetes.io/vsphere-volume" {
		t.Errorf("Wrong name: %s", plug.GetPluginName())
	}

	if !plug.CanSupport(&volumeshadow.Spec{Volume: &v1.Volume{VolumeSource: v1.VolumeSource{VsphereVolume: &v1.VsphereVirtualDiskVolumeSource{}}}}) {
		t.Errorf("Expected true")
	}

	if !plug.CanSupport(&volumeshadow.Spec{PersistentVolume: &v1.PersistentVolume{Spec: v1.PersistentVolumeSpec{PersistentVolumeSource: v1.PersistentVolumeSource{VsphereVolume: &v1.VsphereVirtualDiskVolumeSource{}}}}}) {
		t.Errorf("Expected true")
	}
}

type fakePDManager struct {
}

func getFakeDeviceName(host volumeshadow.VolumeHost, volPath string) string {
	return path.Join(host.GetPluginDir(vsphereVolumePluginName), "device", volPath)
}

func (fake *fakePDManager) CreateVolume(v *vsphereVolumeProvisioner) (vmDiskPath string, volumeSizeKB int, err error) {
	return "[local] test-volume-name.vmdk", 100, nil
}

func (fake *fakePDManager) DeleteVolume(vd *vsphereVolumeDeleter) error {
	if vd.volPath != "[local] test-volume-name.vmdk" {
		return fmt.Errorf("Deleter got unexpected volume path: %s", vd.volPath)
	}
	return nil
}

func TestPlugin(t *testing.T) {
	// Initial setup to test volume plugin
	tmpDir, err := utiltesting.MkTmpdir("vsphereVolumeTest")
	if err != nil {
		t.Fatalf("can't make a temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	plugMgr := volumeshadow.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volumetest.NewFakeVolumeHost(tmpDir, nil, nil, "" /* rootContext */))

	plug, err := plugMgr.FindPluginByName("kubernetes.io/vsphere-volume")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}

	spec := &v1.Volume{
		Name: "vol1",
		VolumeSource: v1.VolumeSource{
			VsphereVolume: &v1.VsphereVirtualDiskVolumeSource{
				VolumePath: "[local] test-volume-name.vmdk",
				FSType:     "ext4",
			},
		},
	}

	// Test Mounter
	fakeManager := &fakePDManager{}
	fakeMounter := &mount.FakeMounter{}
	mounter, err := plug.(*vsphereVolumePlugin).newMounterInternal(volumeshadow.NewSpecFromVolume(spec), types.UID("poduid"), fakeManager, fakeMounter)
	if err != nil {
		t.Errorf("Failed to make a new Mounter: %v", err)
	}
	if mounter == nil {
		t.Errorf("Got a nil Mounter")
	}

	mntPath := path.Join(tmpDir, "pods/poduid/volumes/kubernetes.io~vsphere-volume/vol1")
	path := mounter.GetPath()
	if path != mntPath {
		t.Errorf("Got unexpected path: %s", path)
	}

	if err := mounter.SetUp(nil); err != nil {
		t.Errorf("Expected success, got: %v", err)
	}

	// Test Unmounter
	fakeManager = &fakePDManager{}
	unmounter, err := plug.(*vsphereVolumePlugin).newUnmounterInternal("vol1", types.UID("poduid"), fakeManager, fakeMounter)
	if err != nil {
		t.Errorf("Failed to make a new Unmounter: %v", err)
	}
	if unmounter == nil {
		t.Errorf("Got a nil Unmounter")
	}

	if err := unmounter.TearDown(); err != nil {
		t.Errorf("Expected success, got: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("TearDown() failed, volume path still exists: %s", path)
	} else if !os.IsNotExist(err) {
		t.Errorf("SetUp() failed: %v", err)
	}

	// Test Provisioner
	cap := resource.MustParse("100Mi")
	options := volumeshadow.VolumeOptions{
		Capacity: cap,
		AccessModes: []v1.PersistentVolumeAccessMode{
			v1.ReadWriteOnce,
		},
		PersistentVolumeReclaimPolicy: v1.PersistentVolumeReclaimDelete,
	}
	provisioner, err := plug.(*vsphereVolumePlugin).newProvisionerInternal(options, &fakePDManager{})
	persistentSpec, err := provisioner.Provision()
	if err != nil {
		t.Errorf("Provision() failed: %v", err)
	}

	if persistentSpec.Spec.PersistentVolumeSource.VsphereVolume.VolumePath != "[local] test-volume-name.vmdk" {
		t.Errorf("Provision() returned unexpected path %s", persistentSpec.Spec.PersistentVolumeSource.VsphereVolume.VolumePath)
	}

	cap = persistentSpec.Spec.Capacity[v1.ResourceStorage]
	size := cap.Value()
	if size != 100*1024 {
		t.Errorf("Provision() returned unexpected volume size: %v", size)
	}

	// Test Deleter
	volSpec := &volumeshadow.Spec{
		PersistentVolume: persistentSpec,
	}
	deleter, err := plug.(*vsphereVolumePlugin).newDeleterInternal(volSpec, &fakePDManager{})
	err = deleter.Delete()
	if err != nil {
		t.Errorf("Deleter() failed: %v", err)
	}
}
