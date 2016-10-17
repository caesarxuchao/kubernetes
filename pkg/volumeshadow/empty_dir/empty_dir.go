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

package empty_dir

import (
	"fmt"
	"os"
	"path"

	"github.com/golang/glog"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/types"
	"k8s.io/kubernetes/pkg/util/mount"
	"k8s.io/kubernetes/pkg/util/strings"
	"k8s.io/kubernetes/pkg/volumeshadow"
	volumeutil "k8s.io/kubernetes/pkg/volumeshadow/util"
)

// TODO: in the near future, this will be changed to be more restrictive
// and the group will be set to allow containers to use emptyDir volumes
// from the group attribute.
//
// http://issue.k8s.io/2630
const perm os.FileMode = 0777

// This is the primary entrypoint for volume plugins.
func ProbeVolumePlugins() []volumeshadow.VolumePlugin {
	return []volumeshadow.VolumePlugin{
		&emptyDirPlugin{nil},
	}
}

type emptyDirPlugin struct {
	host volumeshadow.VolumeHost
}

var _ volumeshadow.VolumePlugin = &emptyDirPlugin{}

const (
	emptyDirPluginName = "kubernetes.io/empty-dir"
)

func getPath(uid types.UID, volName string, host volumeshadow.VolumeHost) string {
	return host.GetPodVolumeDir(uid, strings.EscapeQualifiedNameForDisk(emptyDirPluginName), volName)
}

func (plugin *emptyDirPlugin) Init(host volumeshadow.VolumeHost) error {
	plugin.host = host

	return nil
}

func (plugin *emptyDirPlugin) GetPluginName() string {
	return emptyDirPluginName
}

func (plugin *emptyDirPlugin) GetVolumeName(spec *volumeshadow.Spec) (string, error) {
	volumeSource, _ := getVolumeSource(spec)
	if volumeSource == nil {
		return "", fmt.Errorf("Spec does not reference an EmptyDir volume type")
	}

	// Return user defined volume name, since this is an ephemeral volume type
	return spec.Name(), nil
}

func (plugin *emptyDirPlugin) CanSupport(spec *volumeshadow.Spec) bool {
	if spec.Volume != nil && spec.Volume.EmptyDir != nil {
		return true
	}
	return false
}

func (plugin *emptyDirPlugin) RequiresRemount() bool {
	return false
}

func (plugin *emptyDirPlugin) NewMounter(spec *volumeshadow.Spec, pod *v1.Pod, opts volumeshadow.VolumeOptions) (volumeshadow.Mounter, error) {
	return plugin.newMounterInternal(spec, pod, plugin.host.GetMounter(), &realMountDetector{plugin.host.GetMounter()}, opts)
}

func (plugin *emptyDirPlugin) newMounterInternal(spec *volumeshadow.Spec, pod *v1.Pod, mounter mount.Interface, mountDetector mountDetector, opts volumeshadow.VolumeOptions) (volumeshadow.Mounter, error) {
	medium := v1.StorageMediumDefault
	if spec.Volume.EmptyDir != nil { // Support a non-specified source as EmptyDir.
		medium = spec.Volume.EmptyDir.Medium
	}
	return &emptyDir{
		pod:             pod,
		volName:         spec.Name(),
		medium:          medium,
		mounter:         mounter,
		mountDetector:   mountDetector,
		plugin:          plugin,
		rootContext:     plugin.host.GetRootContext(),
		MetricsProvider: volumeshadow.NewMetricsDu(getPath(pod.UID, spec.Name(), plugin.host)),
	}, nil
}

func (plugin *emptyDirPlugin) NewUnmounter(volName string, podUID types.UID) (volumeshadow.Unmounter, error) {
	// Inject real implementations here, test through the internal function.
	return plugin.newUnmounterInternal(volName, podUID, plugin.host.GetMounter(), &realMountDetector{plugin.host.GetMounter()})
}

func (plugin *emptyDirPlugin) newUnmounterInternal(volName string, podUID types.UID, mounter mount.Interface, mountDetector mountDetector) (volumeshadow.Unmounter, error) {
	ed := &emptyDir{
		pod:             &v1.Pod{ObjectMeta: v1.ObjectMeta{UID: podUID}},
		volName:         volName,
		medium:          v1.StorageMediumDefault,
		mounter:         mounter,
		mountDetector:   mountDetector,
		plugin:          plugin,
		MetricsProvider: volumeshadow.NewMetricsDu(getPath(podUID, volName, plugin.host)),
	}
	return ed, nil
}

func (plugin *emptyDirPlugin) ConstructVolumeSpec(volName, mountPath string) (*volumeshadow.Spec, error) {
	emptyDirVolume := &v1.Volume{
		Name: volName,
		VolumeSource: v1.VolumeSource{
			EmptyDir: &v1.EmptyDirVolumeSource{},
		},
	}
	return volumeshadow.NewSpecFromVolume(emptyDirVolume), nil
}

// mountDetector abstracts how to find what kind of mount a path is backed by.
type mountDetector interface {
	// GetMountMedium determines what type of medium a given path is backed
	// by and whether that path is a mount point.  For example, if this
	// returns (mediumMemory, false, nil), the caller knows that the path is
	// on a memory FS (tmpfs on Linux) but is not the root mountpoint of
	// that tmpfs.
	GetMountMedium(path string) (storageMedium, bool, error)
}

type storageMedium int

const (
	mediumUnknown storageMedium = 0 // assume anything we don't explicitly handle is this
	mediumMemory  storageMedium = 1 // memory (e.g. tmpfs on linux)
)

// EmptyDir volumes are temporary directories exposed to the pod.
// These do not persist beyond the lifetime of a pod.
type emptyDir struct {
	pod           *v1.Pod
	volName       string
	medium        v1.StorageMedium
	mounter       mount.Interface
	mountDetector mountDetector
	plugin        *emptyDirPlugin
	rootContext   string
	volumeshadow.MetricsProvider
}

func (ed *emptyDir) GetAttributes() volumeshadow.Attributes {
	return volumeshadow.Attributes{
		ReadOnly:        false,
		Managed:         true,
		SupportsSELinux: true,
	}
}

// SetUp creates new directory.
func (ed *emptyDir) SetUp(fsGroup *int64) error {
	return ed.SetUpAt(ed.GetPath(), fsGroup)
}

// SetUpAt creates new directory.
func (ed *emptyDir) SetUpAt(dir string, fsGroup *int64) error {
	notMnt, err := ed.mounter.IsLikelyNotMountPoint(dir)
	// Getting an os.IsNotExist err from is a contingency; the directory
	// may not exist yet, in which case, setup should run.
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// If the plugin readiness file is present for this volume, and the
	// storage medium is the default, then the volume is ready.  If the
	// medium is memory, and a mountpoint is present, then the volume is
	// ready.
	if volumeutil.IsReady(ed.getMetaDir()) {
		if ed.medium == v1.StorageMediumMemory && !notMnt {
			return nil
		} else if ed.medium == v1.StorageMediumDefault {
			return nil
		}
	}

	// Determine the effective SELinuxOptions to use for this volume.
	securityContext := ""
	if selinuxEnabled() {
		securityContext = ed.rootContext
	}

	switch ed.medium {
	case v1.StorageMediumDefault:
		err = ed.setupDir(dir)
	case v1.StorageMediumMemory:
		err = ed.setupTmpfs(dir, securityContext)
	default:
		err = fmt.Errorf("unknown storage medium %q", ed.medium)
	}

	volumeshadow.SetVolumeOwnership(ed, fsGroup)

	if err == nil {
		volumeutil.SetReady(ed.getMetaDir())
	}

	return err
}

// setupTmpfs creates a tmpfs mount at the specified directory with the
// specified SELinux context.
func (ed *emptyDir) setupTmpfs(dir string, selinux string) error {
	if ed.mounter == nil {
		return fmt.Errorf("memory storage requested, but mounter is nil")
	}
	if err := ed.setupDir(dir); err != nil {
		return err
	}
	// Make SetUp idempotent.
	medium, isMnt, err := ed.mountDetector.GetMountMedium(dir)
	if err != nil {
		return err
	}
	// If the directory is a mountpoint with medium memory, there is no
	// work to do since we are already in the desired state.
	if isMnt && medium == mediumMemory {
		return nil
	}

	// By default a tmpfs mount will receive a different SELinux context
	// which is not readable from the SELinux context of a docker container.
	var opts []string
	if selinux != "" {
		opts = []string{fmt.Sprintf("rootcontext=\"%v\"", selinux)}
	} else {
		opts = []string{}
	}

	glog.V(3).Infof("pod %v: mounting tmpfs for volume %v with opts %v", ed.pod.UID, ed.volName, opts)
	return ed.mounter.Mount("tmpfs", dir, "tmpfs", opts)
}

// setupDir creates the directory with the specified SELinux context and
// the default permissions specified by the perm constant.
func (ed *emptyDir) setupDir(dir string) error {
	// Create the directory if it doesn't already exist.
	if err := os.MkdirAll(dir, perm); err != nil {
		return err
	}

	// stat the directory to read permission bits
	fileinfo, err := os.Lstat(dir)
	if err != nil {
		return err
	}

	if fileinfo.Mode().Perm() != perm.Perm() {
		// If the permissions on the created directory are wrong, the
		// kubelet is probably running with a umask set.  In order to
		// avoid clearing the umask for the entire process or locking
		// the thread, clearing the umask, creating the dir, restoring
		// the umask, and unlocking the thread, we do a chmod to set
		// the specific bits we need.
		err := os.Chmod(dir, perm)
		if err != nil {
			return err
		}

		fileinfo, err = os.Lstat(dir)
		if err != nil {
			return err
		}

		if fileinfo.Mode().Perm() != perm.Perm() {
			glog.Errorf("Expected directory %q permissions to be: %s; got: %s", dir, perm.Perm(), fileinfo.Mode().Perm())
		}
	}

	return nil
}

func (ed *emptyDir) GetPath() string {
	return getPath(ed.pod.UID, ed.volName, ed.plugin.host)
}

// TearDown simply discards everything in the directory.
func (ed *emptyDir) TearDown() error {
	return ed.TearDownAt(ed.GetPath())
}

// TearDownAt simply discards everything in the directory.
func (ed *emptyDir) TearDownAt(dir string) error {
	// Figure out the medium.
	medium, isMnt, err := ed.mountDetector.GetMountMedium(dir)
	if err != nil {
		return err
	}
	if isMnt && medium == mediumMemory {
		ed.medium = v1.StorageMediumMemory
		return ed.teardownTmpfs(dir)
	}
	// assume StorageMediumDefault
	return ed.teardownDefault(dir)
}

func (ed *emptyDir) teardownDefault(dir string) error {
	tmpDir, err := volumeshadow.RenameDirectory(dir, ed.volName+".deleting~")
	if err != nil {
		return err
	}
	err = os.RemoveAll(tmpDir)
	if err != nil {
		return err
	}
	return nil
}

func (ed *emptyDir) teardownTmpfs(dir string) error {
	if ed.mounter == nil {
		return fmt.Errorf("memory storage requested, but mounter is nil")
	}
	if err := ed.mounter.Unmount(dir); err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	return nil
}

func (ed *emptyDir) getMetaDir() string {
	return path.Join(ed.plugin.host.GetPodPluginDir(ed.pod.UID, strings.EscapeQualifiedNameForDisk(emptyDirPluginName)), ed.volName)
}

func getVolumeSource(spec *volumeshadow.Spec) (*v1.EmptyDirVolumeSource, bool) {
	var readOnly bool
	var volumeSource *v1.EmptyDirVolumeSource

	if spec.Volume != nil && spec.Volume.EmptyDir != nil {
		volumeSource = spec.Volume.EmptyDir
		readOnly = spec.ReadOnly
	}

	return volumeSource, readOnly
}
