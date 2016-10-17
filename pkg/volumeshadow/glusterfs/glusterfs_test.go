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

package glusterfs

import (
	"fmt"
	"os"
	"testing"

	"k8s.io/client-go/1.5/kubernetes/fake"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/types"
	"k8s.io/kubernetes/pkg/util/exec"
	"k8s.io/kubernetes/pkg/util/mount"
	utiltesting "k8s.io/kubernetes/pkg/util/testing"
	"k8s.io/kubernetes/pkg/volumeshadow"
	volumetest "k8s.io/kubernetes/pkg/volumeshadow/testing"
)

func TestCanSupport(t *testing.T) {
	tmpDir, err := utiltesting.MkTmpdir("glusterfs_test")
	if err != nil {
		t.Fatalf("error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	plugMgr := volumeshadow.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volumetest.NewFakeVolumeHost(tmpDir, nil, nil, "" /* rootContext */))
	plug, err := plugMgr.FindPluginByName("kubernetes.io/glusterfs")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}
	if plug.GetPluginName() != "kubernetes.io/glusterfs" {
		t.Errorf("Wrong name: %s", plug.GetPluginName())
	}
	if plug.CanSupport(&volumeshadow.Spec{PersistentVolume: &v1.PersistentVolume{Spec: v1.PersistentVolumeSpec{PersistentVolumeSource: v1.PersistentVolumeSource{}}}}) {
		t.Errorf("Expected false")
	}
	if plug.CanSupport(&volumeshadow.Spec{Volume: &v1.Volume{VolumeSource: v1.VolumeSource{}}}) {
		t.Errorf("Expected false")
	}
}

func TestGetAccessModes(t *testing.T) {
	tmpDir, err := utiltesting.MkTmpdir("glusterfs_test")
	if err != nil {
		t.Fatalf("error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	plugMgr := volumeshadow.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volumetest.NewFakeVolumeHost(tmpDir, nil, nil, "" /* rootContext */))

	plug, err := plugMgr.FindPersistentPluginByName("kubernetes.io/glusterfs")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}
	if !contains(plug.GetAccessModes(), v1.ReadWriteOnce) || !contains(plug.GetAccessModes(), v1.ReadOnlyMany) || !contains(plug.GetAccessModes(), v1.ReadWriteMany) {
		t.Errorf("Expected three AccessModeTypes:  %s, %s, and %s", v1.ReadWriteOnce, v1.ReadOnlyMany, v1.ReadWriteMany)
	}
}

func contains(modes []v1.PersistentVolumeAccessMode, mode v1.PersistentVolumeAccessMode) bool {
	for _, m := range modes {
		if m == mode {
			return true
		}
	}
	return false
}

func doTestPlugin(t *testing.T, spec *volumeshadow.Spec) {
	tmpDir, err := utiltesting.MkTmpdir("glusterfs_test")
	if err != nil {
		t.Fatalf("error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	plugMgr := volumeshadow.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volumetest.NewFakeVolumeHost(tmpDir, nil, nil, "" /* rootContext */))
	plug, err := plugMgr.FindPluginByName("kubernetes.io/glusterfs")
	if err != nil {
		t.Errorf("Can't find the plugin by name")
	}
	ep := &v1.Endpoints{ObjectMeta: v1.ObjectMeta{Name: "foo"}, Subsets: []v1.EndpointSubset{{
		Addresses: []v1.EndpointAddress{{IP: "127.0.0.1"}}}}}
	var fcmd exec.FakeCmd
	fcmd = exec.FakeCmd{
		CombinedOutputScript: []exec.FakeCombinedOutputAction{
			// mount
			func() ([]byte, error) {
				return []byte{}, nil
			},
		},
	}
	fake := exec.FakeExec{
		CommandScript: []exec.FakeCommandAction{
			func(cmd string, args ...string) exec.Cmd { return exec.InitFakeCmd(&fcmd, cmd, args...) },
		},
	}
	pod := &v1.Pod{ObjectMeta: v1.ObjectMeta{UID: types.UID("poduid")}}
	mounter, err := plug.(*glusterfsPlugin).newMounterInternal(spec, ep, pod, &mount.FakeMounter{}, &fake)
	volumePath := mounter.GetPath()
	if err != nil {
		t.Errorf("Failed to make a new Mounter: %v", err)
	}
	if mounter == nil {
		t.Error("Got a nil Mounter")
	}
	path := mounter.GetPath()
	expectedPath := fmt.Sprintf("%s/pods/poduid/volumes/kubernetes.io~glusterfs/vol1", tmpDir)
	if path != expectedPath {
		t.Errorf("Unexpected path, expected %q, got: %q", expectedPath, path)
	}
	if err := mounter.SetUp(nil); err != nil {
		t.Errorf("Expected success, got: %v", err)
	}
	if _, err := os.Stat(volumePath); err != nil {
		if os.IsNotExist(err) {
			t.Errorf("SetUp() failed, volume path not created: %s", volumePath)
		} else {
			t.Errorf("SetUp() failed: %v", err)
		}
	}
	unmounter, err := plug.(*glusterfsPlugin).newUnmounterInternal("vol1", types.UID("poduid"), &mount.FakeMounter{})
	if err != nil {
		t.Errorf("Failed to make a new Unmounter: %v", err)
	}
	if unmounter == nil {
		t.Error("Got a nil Unmounter")
	}
	if err := unmounter.TearDown(); err != nil {
		t.Errorf("Expected success, got: %v", err)
	}
	if _, err := os.Stat(volumePath); err == nil {
		t.Errorf("TearDown() failed, volume path still exists: %s", volumePath)
	} else if !os.IsNotExist(err) {
		t.Errorf("SetUp() failed: %v", err)
	}
}

func TestPluginVolume(t *testing.T) {
	vol := &v1.Volume{
		Name:         "vol1",
		VolumeSource: v1.VolumeSource{Glusterfs: &v1.GlusterfsVolumeSource{EndpointsName: "ep", Path: "vol", ReadOnly: false}},
	}
	doTestPlugin(t, volumeshadow.NewSpecFromVolume(vol))
}

func TestPluginPersistentVolume(t *testing.T) {
	vol := &v1.PersistentVolume{
		ObjectMeta: v1.ObjectMeta{
			Name: "vol1",
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				Glusterfs: &v1.GlusterfsVolumeSource{EndpointsName: "ep", Path: "vol", ReadOnly: false},
			},
		},
	}

	doTestPlugin(t, volumeshadow.NewSpecFromPersistentVolume(vol, false))
}

func TestPersistentClaimReadOnlyFlag(t *testing.T) {
	tmpDir, err := utiltesting.MkTmpdir("glusterfs_test")
	if err != nil {
		t.Fatalf("error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pv := &v1.PersistentVolume{
		ObjectMeta: v1.ObjectMeta{
			Name: "pvA",
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeSource: v1.PersistentVolumeSource{
				Glusterfs: &v1.GlusterfsVolumeSource{EndpointsName: "ep", Path: "vol", ReadOnly: false},
			},
			ClaimRef: &v1.ObjectReference{
				Name: "claimA",
			},
		},
	}

	claim := &v1.PersistentVolumeClaim{
		ObjectMeta: v1.ObjectMeta{
			Name:      "claimA",
			Namespace: "nsA",
		},
		Spec: v1.PersistentVolumeClaimSpec{
			VolumeName: "pvA",
		},
		Status: v1.PersistentVolumeClaimStatus{
			Phase: v1.ClaimBound,
		},
	}

	ep := &v1.Endpoints{
		ObjectMeta: v1.ObjectMeta{
			Namespace: "nsA",
			Name:      "ep",
		},
		Subsets: []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{IP: "127.0.0.1"}},
			Ports:     []v1.EndpointPort{{Name: "foo", Port: 80, Protocol: v1.ProtocolTCP}},
		}},
	}

	client := fake.NewSimpleClientset(pv, claim, ep)

	plugMgr := volumeshadow.VolumePluginMgr{}
	plugMgr.InitPlugins(ProbeVolumePlugins(), volumetest.NewFakeVolumeHost(tmpDir, client, nil, "" /* rootContext */))
	plug, _ := plugMgr.FindPluginByName(glusterfsPluginName)

	// readOnly bool is supplied by persistent-claim volume source when its mounter creates other volumes
	spec := volumeshadow.NewSpecFromPersistentVolume(pv, true)
	pod := &v1.Pod{ObjectMeta: v1.ObjectMeta{Namespace: "nsA", UID: types.UID("poduid")}}
	mounter, _ := plug.NewMounter(spec, pod, volumeshadow.VolumeOptions{})

	if !mounter.GetAttributes().ReadOnly {
		t.Errorf("Expected true for mounter.IsReadOnly")
	}
}

func TestAnnotations(t *testing.T) {
	// Pass a provisioningConfigs through paramToAnnotations and back through
	// annotationsToParam and check it did not change in the process.
	tests := []provisioningConfig{
		{
		// Everything empty
		},
		{
			// Everything with a value
			url:             "http://localhost",
			user:            "admin",
			secretNamespace: "default",
			secretName:      "gluster-secret",
			userKey:         "mykey",
		},
		{
			// No secret
			url:             "http://localhost",
			user:            "admin",
			secretNamespace: "",
			secretName:      "",
			userKey:         "",
		},
	}

	for i, test := range tests {
		provisioner := &glusterfsVolumeProvisioner{
			provisioningConfig: test,
		}
		deleter := &glusterfsVolumeDeleter{}

		pv := &v1.PersistentVolume{
			ObjectMeta: v1.ObjectMeta{
				Name: "pv",
			},
		}

		provisioner.paramToAnnotations(pv)
		err := deleter.annotationsToParam(pv)
		if err != nil {
			t.Errorf("test %d failed: %v", i, err)
		}
		if test.url != deleter.url {
			t.Errorf("test %d failed: expected url %q, got %q", i, test.url, deleter.url)
		}
		if test.user != deleter.user {
			t.Errorf("test %d failed: expected user %q, got %q", i, test.user, deleter.user)
		}
		if test.userKey != deleter.userKey {
			t.Errorf("test %d failed: expected userKey %q, got %q", i, test.userKey, deleter.userKey)
		}
		if test.secretNamespace != deleter.secretNamespace {
			t.Errorf("test %d failed: expected secretNamespace %q, got %q", i, test.secretNamespace, deleter.secretNamespace)
		}
		if test.secretName != deleter.secretName {
			t.Errorf("test %d failed: expected secretName %q, got %q", i, test.secretName, deleter.secretName)
		}
	}
}
