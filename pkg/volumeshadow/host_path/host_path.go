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

package host_path

import (
	"fmt"
	"os"
	"regexp"

	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/types"
	"k8s.io/kubernetes/pkg/util/uuid"
	"k8s.io/kubernetes/pkg/volumeshadow"
)

// This is the primary entrypoint for volume plugins.
// The volumeConfig arg provides the ability to configure volume behavior.  It is implemented as a pointer to allow nils.
// The hostPathPlugin is used to store the volumeConfig and give it, when needed, to the func that creates HostPath Recyclers.
// Tests that exercise recycling should not use this func but instead use ProbeRecyclablePlugins() to override default behavior.
func ProbeVolumePlugins(volumeConfig volumeshadow.VolumeConfig) []volumeshadow.VolumePlugin {
	return []volumeshadow.VolumePlugin{
		&hostPathPlugin{
			host:               nil,
			newRecyclerFunc:    newRecycler,
			newDeleterFunc:     newDeleter,
			newProvisionerFunc: newProvisioner,
			config:             volumeConfig,
		},
	}
}

type hostPathPlugin struct {
	host volumeshadow.VolumeHost
	// decouple creating Recyclers/Deleters/Provisioners by deferring to a function.  Allows for easier testing.
	newRecyclerFunc    func(pvName string, spec *volumeshadow.Spec, eventRecorder volumeshadow.RecycleEventRecorder, host volumeshadow.VolumeHost, volumeConfig volumeshadow.VolumeConfig) (volumeshadow.Recycler, error)
	newDeleterFunc     func(spec *volumeshadow.Spec, host volumeshadow.VolumeHost) (volumeshadow.Deleter, error)
	newProvisionerFunc func(options volumeshadow.VolumeOptions, host volumeshadow.VolumeHost) (volumeshadow.Provisioner, error)
	config             volumeshadow.VolumeConfig
}

var _ volumeshadow.VolumePlugin = &hostPathPlugin{}
var _ volumeshadow.PersistentVolumePlugin = &hostPathPlugin{}
var _ volumeshadow.RecyclableVolumePlugin = &hostPathPlugin{}
var _ volumeshadow.DeletableVolumePlugin = &hostPathPlugin{}
var _ volumeshadow.ProvisionableVolumePlugin = &hostPathPlugin{}

const (
	hostPathPluginName = "kubernetes.io/host-path"
)

func (plugin *hostPathPlugin) Init(host volumeshadow.VolumeHost) error {
	plugin.host = host
	return nil
}

func (plugin *hostPathPlugin) GetPluginName() string {
	return hostPathPluginName
}

func (plugin *hostPathPlugin) GetVolumeName(spec *volumeshadow.Spec) (string, error) {
	volumeSource, _, err := getVolumeSource(spec)
	if err != nil {
		return "", err
	}

	return volumeSource.Path, nil
}

func (plugin *hostPathPlugin) CanSupport(spec *volumeshadow.Spec) bool {
	return (spec.PersistentVolume != nil && spec.PersistentVolume.Spec.HostPath != nil) ||
		(spec.Volume != nil && spec.Volume.HostPath != nil)
}

func (plugin *hostPathPlugin) RequiresRemount() bool {
	return false
}

func (plugin *hostPathPlugin) GetAccessModes() []v1.PersistentVolumeAccessMode {
	return []v1.PersistentVolumeAccessMode{
		v1.ReadWriteOnce,
	}
}

func (plugin *hostPathPlugin) NewMounter(spec *volumeshadow.Spec, pod *v1.Pod, _ volumeshadow.VolumeOptions) (volumeshadow.Mounter, error) {
	hostPathVolumeSource, readOnly, err := getVolumeSource(spec)
	if err != nil {
		return nil, err
	}
	return &hostPathMounter{
		hostPath: &hostPath{path: hostPathVolumeSource.Path},
		readOnly: readOnly,
	}, nil
}

func (plugin *hostPathPlugin) NewUnmounter(volName string, podUID types.UID) (volumeshadow.Unmounter, error) {
	return &hostPathUnmounter{&hostPath{
		path: "",
	}}, nil
}

func (plugin *hostPathPlugin) NewRecycler(pvName string, spec *volumeshadow.Spec, eventRecorder volumeshadow.RecycleEventRecorder) (volumeshadow.Recycler, error) {
	return plugin.newRecyclerFunc(pvName, spec, eventRecorder, plugin.host, plugin.config)
}

func (plugin *hostPathPlugin) NewDeleter(spec *volumeshadow.Spec) (volumeshadow.Deleter, error) {
	return plugin.newDeleterFunc(spec, plugin.host)
}

func (plugin *hostPathPlugin) NewProvisioner(options volumeshadow.VolumeOptions) (volumeshadow.Provisioner, error) {
	if !plugin.config.ProvisioningEnabled {
		return nil, fmt.Errorf("Provisioning in volume plugin %q is disabled", plugin.GetPluginName())
	}
	if len(options.AccessModes) == 0 {
		options.AccessModes = plugin.GetAccessModes()
	}
	return plugin.newProvisionerFunc(options, plugin.host)
}

func (plugin *hostPathPlugin) ConstructVolumeSpec(volumeName, mountPath string) (*volumeshadow.Spec, error) {
	hostPathVolume := &v1.Volume{
		Name: volumeName,
		VolumeSource: v1.VolumeSource{
			HostPath: &v1.HostPathVolumeSource{
				Path: volumeName,
			},
		},
	}
	return volumeshadow.NewSpecFromVolume(hostPathVolume), nil
}

func newRecycler(pvName string, spec *volumeshadow.Spec, eventRecorder volumeshadow.RecycleEventRecorder, host volumeshadow.VolumeHost, config volumeshadow.VolumeConfig) (volumeshadow.Recycler, error) {
	if spec.PersistentVolume == nil || spec.PersistentVolume.Spec.HostPath == nil {
		return nil, fmt.Errorf("spec.PersistentVolumeSource.HostPath is nil")
	}
	path := spec.PersistentVolume.Spec.HostPath.Path
	return &hostPathRecycler{
		name:          spec.Name(),
		path:          path,
		host:          host,
		config:        config,
		timeout:       volumeshadow.CalculateTimeoutForVolume(config.RecyclerMinimumTimeout, config.RecyclerTimeoutIncrement, spec.PersistentVolume),
		pvName:        pvName,
		eventRecorder: eventRecorder,
	}, nil
}

func newDeleter(spec *volumeshadow.Spec, host volumeshadow.VolumeHost) (volumeshadow.Deleter, error) {
	if spec.PersistentVolume != nil && spec.PersistentVolume.Spec.HostPath == nil {
		return nil, fmt.Errorf("spec.PersistentVolumeSource.HostPath is nil")
	}
	path := spec.PersistentVolume.Spec.HostPath.Path
	return &hostPathDeleter{name: spec.Name(), path: path, host: host}, nil
}

func newProvisioner(options volumeshadow.VolumeOptions, host volumeshadow.VolumeHost) (volumeshadow.Provisioner, error) {
	return &hostPathProvisioner{options: options, host: host}, nil
}

// HostPath volumes represent a bare host file or directory mount.
// The direct at the specified path will be directly exposed to the container.
type hostPath struct {
	path string
	volumeshadow.MetricsNil
}

func (hp *hostPath) GetPath() string {
	return hp.path
}

type hostPathMounter struct {
	*hostPath
	readOnly bool
}

var _ volumeshadow.Mounter = &hostPathMounter{}

func (b *hostPathMounter) GetAttributes() volumeshadow.Attributes {
	return volumeshadow.Attributes{
		ReadOnly:        b.readOnly,
		Managed:         false,
		SupportsSELinux: false,
	}
}

// SetUp does nothing.
func (b *hostPathMounter) SetUp(fsGroup *int64) error {
	return nil
}

// SetUpAt does not make sense for host paths - probably programmer error.
func (b *hostPathMounter) SetUpAt(dir string, fsGroup *int64) error {
	return fmt.Errorf("SetUpAt() does not make sense for host paths")
}

func (b *hostPathMounter) GetPath() string {
	return b.path
}

type hostPathUnmounter struct {
	*hostPath
}

var _ volumeshadow.Unmounter = &hostPathUnmounter{}

// TearDown does nothing.
func (c *hostPathUnmounter) TearDown() error {
	return nil
}

// TearDownAt does not make sense for host paths - probably programmer error.
func (c *hostPathUnmounter) TearDownAt(dir string) error {
	return fmt.Errorf("TearDownAt() does not make sense for host paths")
}

// hostPathRecycler implements a Recycler for the HostPath plugin
// This implementation is meant for testing only and only works in a single node cluster
type hostPathRecycler struct {
	name    string
	path    string
	host    volumeshadow.VolumeHost
	config  volumeshadow.VolumeConfig
	timeout int64
	volumeshadow.MetricsNil
	pvName        string
	eventRecorder volumeshadow.RecycleEventRecorder
}

func (r *hostPathRecycler) GetPath() string {
	return r.path
}

// Recycle recycles/scrubs clean a HostPath volume.
// Recycle blocks until the pod has completed or any error occurs.
// HostPath recycling only works in single node clusters and is meant for testing purposes only.
func (r *hostPathRecycler) Recycle() error {
	pod := r.config.RecyclerPodTemplate
	// overrides
	pod.Spec.ActiveDeadlineSeconds = &r.timeout
	pod.Spec.Volumes[0].VolumeSource = v1.VolumeSource{
		HostPath: &v1.HostPathVolumeSource{
			Path: r.path,
		},
	}
	return volumeshadow.RecycleVolumeByWatchingPodUntilCompletion(r.pvName, pod, r.host.GetKubeClient(), r.eventRecorder)
}

// hostPathProvisioner implements a Provisioner for the HostPath plugin
// This implementation is meant for testing only and only works in a single node cluster.
type hostPathProvisioner struct {
	host    volumeshadow.VolumeHost
	options volumeshadow.VolumeOptions
}

// Create for hostPath simply creates a local /tmp/hostpath_pv/%s directory as a new PersistentVolume.
// This Provisioner is meant for development and testing only and WILL NOT WORK in a multi-node cluster.
func (r *hostPathProvisioner) Provision() (*v1.PersistentVolume, error) {
	fullpath := fmt.Sprintf("/tmp/hostpath_pv/%s", uuid.NewUUID())

	pv := &v1.PersistentVolume{
		ObjectMeta: v1.ObjectMeta{
			Name: r.options.PVName,
			Annotations: map[string]string{
				"kubernetes.io/createdby": "hostpath-dynamic-provisioner",
			},
		},
		Spec: v1.PersistentVolumeSpec{
			PersistentVolumeReclaimPolicy: r.options.PersistentVolumeReclaimPolicy,
			AccessModes:                   r.options.AccessModes,
			Capacity: v1.ResourceList{
				v1.ResourceName(v1.ResourceStorage): r.options.Capacity,
			},
			PersistentVolumeSource: v1.PersistentVolumeSource{
				HostPath: &v1.HostPathVolumeSource{
					Path: fullpath,
				},
			},
		},
	}

	return pv, os.MkdirAll(pv.Spec.HostPath.Path, 0750)
}

// hostPathDeleter deletes a hostPath PV from the cluster.
// This deleter only works on a single host cluster and is for testing purposes only.
type hostPathDeleter struct {
	name string
	path string
	host volumeshadow.VolumeHost
	volumeshadow.MetricsNil
}

func (r *hostPathDeleter) GetPath() string {
	return r.path
}

// Delete for hostPath removes the local directory so long as it is beneath /tmp/*.
// THIS IS FOR TESTING AND LOCAL DEVELOPMENT ONLY!  This message should scare you away from using
// this deleter for anything other than development and testing.
func (r *hostPathDeleter) Delete() error {
	regexp := regexp.MustCompile("/tmp/.+")
	if !regexp.MatchString(r.GetPath()) {
		return fmt.Errorf("host_path deleter only supports /tmp/.+ but received provided %s", r.GetPath())
	}
	return os.RemoveAll(r.GetPath())
}

func getVolumeSource(
	spec *volumeshadow.Spec) (*v1.HostPathVolumeSource, bool, error) {
	if spec.Volume != nil && spec.Volume.HostPath != nil {
		return spec.Volume.HostPath, spec.ReadOnly, nil
	} else if spec.PersistentVolume != nil &&
		spec.PersistentVolume.Spec.HostPath != nil {
		return spec.PersistentVolume.Spec.HostPath, spec.ReadOnly, nil
	}

	return nil, false, fmt.Errorf("Spec does not reference an HostPath volume type")
}
