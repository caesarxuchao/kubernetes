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

package podgc

import (
	"sort"
	"sync"
	"time"

	clientset "k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/tools/cache"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/controller/informers"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/metrics"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/util/wait"

	"github.com/golang/glog"
)

const (
	gcCheckPeriod = 20 * time.Second
)

type PodGCController struct {
	kubeClient clientset.Interface

	// internalPodInformer is used to hold a personal informer.  If we're using
	// a normal shared informer, then the informer will be started for us.  If
	// we have a personal informer, we must start it ourselves.   If you start
	// the controller using NewPodGC(..., passing SharedInformer, ...), this
	// will be null
	internalPodInformer cache.SharedIndexInformer

	podStore  cache.StoreToPodLister
	nodeStore cache.StoreToNodeLister

	podController  cache.ControllerInterface
	nodeController cache.ControllerInterface

	deletePod              func(namespace, name string) error
	terminatedPodThreshold int
}

func NewPodGC(kubeClient clientset.Interface, podInformer cache.SharedIndexInformer, terminatedPodThreshold int) *PodGCController {
	if kubeClient != nil && kubeClient.Core().GetRESTClient().GetRateLimiter() != nil {
		metrics.RegisterMetricAndTrackRateLimiterUsage("gc_controller", kubeClient.Core().GetRESTClient().GetRateLimiter())
	}
	gcc := &PodGCController{
		kubeClient:             kubeClient,
		terminatedPodThreshold: terminatedPodThreshold,
		deletePod: func(namespace, name string) error {
			return kubeClient.Core().Pods(namespace).Delete(name, v1.NewDeleteOptions(0))
		},
	}

	gcc.podStore.Indexer = podInformer.GetIndexer()
	gcc.podController = podInformer.GetController()

	gcc.nodeStore.Store, gcc.nodeController = cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				return gcc.kubeClient.Core().Nodes().List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				return gcc.kubeClient.Core().Nodes().Watch(options)
			},
		},
		&v1.Node{},
		controller.NoResyncPeriodFunc(),
		cache.ResourceEventHandlerFuncs{},
	)

	return gcc
}

func NewFromClient(
	kubeClient clientset.Interface,
	terminatedPodThreshold int,
) *PodGCController {
	podInformer := informers.NewPodInformer(kubeClient, controller.NoResyncPeriodFunc())
	controller := NewPodGC(kubeClient, podInformer, terminatedPodThreshold)
	controller.internalPodInformer = podInformer
	return controller
}

func (gcc *PodGCController) Run(stop <-chan struct{}) {
	if gcc.internalPodInformer != nil {
		go gcc.podController.Run(stop)
	}
	go gcc.nodeController.Run(stop)
	go wait.Until(gcc.gc, gcCheckPeriod, stop)
	<-stop
}

func (gcc *PodGCController) gc() {
	pods, err := gcc.podStore.List(labels.Everything())
	if err != nil {
		glog.Errorf("Error while listing all Pods: %v", err)
		return
	}
	if gcc.terminatedPodThreshold > 0 {
		gcc.gcTerminated(pods)
	}
	gcc.gcOrphaned(pods)
}

func isPodTerminated(pod *v1.Pod) bool {
	if phase := pod.Status.Phase; phase != v1.PodPending && phase != v1.PodRunning && phase != v1.PodUnknown {
		return true
	}
	return false
}

func (gcc *PodGCController) gcTerminated(pods []*v1.Pod) {
	terminatedPods := []*v1.Pod{}
	for _, pod := range pods {
		if isPodTerminated(pod) {
			terminatedPods = append(terminatedPods, pod)
		}
	}

	terminatedPodCount := len(terminatedPods)
	sort.Sort(byCreationTimestamp(terminatedPods))

	deleteCount := terminatedPodCount - gcc.terminatedPodThreshold

	if deleteCount > terminatedPodCount {
		deleteCount = terminatedPodCount
	}
	if deleteCount > 0 {
		glog.Infof("garbage collecting %v pods", deleteCount)
	}

	var wait sync.WaitGroup
	for i := 0; i < deleteCount; i++ {
		wait.Add(1)
		go func(namespace string, name string) {
			defer wait.Done()
			if err := gcc.deletePod(namespace, name); err != nil {
				// ignore not founds
				defer utilruntime.HandleError(err)
			}
		}(terminatedPods[i].Namespace, terminatedPods[i].Name)
	}
	wait.Wait()
}

// cleanupOrphanedPods deletes pods that are bound to nodes that don't exist.
func (gcc *PodGCController) gcOrphaned(pods []*v1.Pod) {
	glog.V(4).Infof("GC'ing orphaned")

	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			continue
		}
		if _, exists, _ := gcc.nodeStore.GetByKey(pod.Spec.NodeName); exists {
			continue
		}
		glog.V(2).Infof("Found orphaned Pod %v assigned to the Node %v. Deleting.", pod.Name, pod.Spec.NodeName)
		if err := gcc.deletePod(pod.Namespace, pod.Name); err != nil {
			utilruntime.HandleError(err)
		} else {
			glog.V(4).Infof("Forced deletion of oprhaned Pod %s succeeded", pod.Name)
		}
	}
}

// byCreationTimestamp sorts a list by creation timestamp, using their names as a tie breaker.
type byCreationTimestamp []*v1.Pod

func (o byCreationTimestamp) Len() int      { return len(o) }
func (o byCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o byCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(o[j].CreationTimestamp)
}
