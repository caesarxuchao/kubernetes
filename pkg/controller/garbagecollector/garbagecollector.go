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

package garbagecollector

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/errors"
	"k8s.io/kubernetes/pkg/api/meta"
	"k8s.io/kubernetes/pkg/api/meta/metatypes"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/client/cache"
	"k8s.io/kubernetes/pkg/client/typed/dynamic"
	"k8s.io/kubernetes/pkg/controller/garbagecollector/metaonly"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/runtime/schema"
	"k8s.io/kubernetes/pkg/types"
	"k8s.io/kubernetes/pkg/util/clock"
	utilerrors "k8s.io/kubernetes/pkg/util/errors"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/kubernetes/pkg/util/wait"
	"k8s.io/kubernetes/pkg/util/workqueue"
	"k8s.io/kubernetes/pkg/watch"
)

const ResourceResyncTime time.Duration = 0

type monitor struct {
	store      cache.Store
	controller *cache.Controller
}

type objectReference struct {
	metatypes.OwnerReference
	// This is needed by the dynamic client
	Namespace string
}

func (s objectReference) String() string {
	return fmt.Sprintf("[%s/%s, namespace: %s, name: %s, uid: %s]", s.APIVersion, s.Kind, s.Namespace, s.Name, s.UID)
}

// The single-threaded Propagator.processEvent() is the sole writer of the
// nodes. The multi-threaded GarbageCollector.processItem() reads the nodes.
type node struct {
	identity objectReference
	// dependents will be read by the orphan() routine, we need to protect it with a lock.
	dependentsLock sync.RWMutex
	dependents     map[*node]struct{}
	// this is set by processEvent() if the object has non-nil DeletionTimestamp
	// and has the FianlizerDeleteDependents.
	deletingDependents bool
	// this records if the object's deletionTimestamp is non-nil.
	beingDeleted bool
	// when processing an Update event, we need to compare the updated
	// ownerReferences with the owners recorded in the graph.
	owners []metatypes.OwnerReference
}

func (ownerNode *node) addDependent(dependent *node) {
	ownerNode.dependentsLock.Lock()
	defer ownerNode.dependentsLock.Unlock()
	ownerNode.dependents[dependent] = struct{}{}
}

func (ownerNode *node) deleteDependent(dependent *node) {
	ownerNode.dependentsLock.Lock()
	defer ownerNode.dependentsLock.Unlock()
	delete(ownerNode.dependents, dependent)
}

func (n *node) blockingDependents() []*node {
	var ret []*node
	for dep := range n.dependents {
		for _, owner := range dep.owners {
			if owner.UID == n.identity.UID && owner.BlockOwnerDeletion != nil && *owner.BlockOwnerDeletion {
				ret = append(ret, dep)
			}
		}
	}
	return ret
}

// TODO: add a unit test
// generate a patch that unsets the BlockOwnerDeletion field of all
// ownerReferences of node.
func (n *node) unblockOwnerReferences() ([]byte, error) {
	var dummy v1.Pod
	var blockingRefs []metatypes.OwnerReference
	falseVar := false
	for _, owner := range n.owners {
		if owner.BlockOwnerDeletion != nil && *owner.BlockOwnerDeletion {
			ref := owner
			ref.BlockOwnerDeletion = &falseVar
			blockingRefs = append(blockingRefs, ref)
		}
	}
	dummy.ObjectMeta.SetOwnerReferences(blockingRefs)
	dummy.ObjectMeta.UID = n.identity.UID
	return json.Marshal(dummy)
}

type eventType int

const (
	addEvent eventType = iota
	updateEvent
	deleteEvent
)

type event struct {
	eventType eventType
	obj       interface{}
	// the update event comes with an old object, but it's not used by the garbage collector.
	oldObj interface{}
}

type concurrentUIDToNode struct {
	*sync.RWMutex
	uidToNode map[types.UID]*node
}

func (m *concurrentUIDToNode) Write(node *node) {
	m.Lock()
	defer m.Unlock()
	m.uidToNode[node.identity.UID] = node
}

func (m *concurrentUIDToNode) Read(uid types.UID) (*node, bool) {
	m.RLock()
	defer m.RUnlock()
	n, ok := m.uidToNode[uid]
	return n, ok
}

func (m *concurrentUIDToNode) Delete(uid types.UID) {
	m.Lock()
	defer m.Unlock()
	delete(m.uidToNode, uid)
}

type Propagator struct {
	eventQueue *workqueue.TimedWorkQueue
	// uidToNode doesn't require a lock to protect, because only the
	// single-threaded Propagator.processEvent() reads/writes it.
	uidToNode *concurrentUIDToNode
	gc        *GarbageCollector
}

// addDependentToOwners adds n to owners' dependents list. If the owner does not
// exist in the p.uidToNode yet, a "virtual" node will be created to represent
// the owner. The "virtual" node will be enqueued to the dirtyQueue, so that
// processItem() will verify if the owner exists according to the API server.
func (p *Propagator) addDependentToOwners(n *node, owners []metatypes.OwnerReference) {
	for _, owner := range owners {
		ownerNode, ok := p.uidToNode.Read(owner.UID)
		if !ok {
			// Create a "virtual" node in the graph for the owner if it doesn't
			// exist in the graph yet. Then enqueue the virtual node into the
			// dirtyQueue. The garbage processor will enqueue a virtual delete
			// event to delete it from the graph if API server confirms this
			// owner doesn't exist.
			ownerNode = &node{
				identity: objectReference{
					OwnerReference: owner,
					Namespace:      n.identity.Namespace,
				},
				dependents: make(map[*node]struct{}),
			}
			glog.V(6).Infof("add virtual node.identity: %s\n\n", ownerNode.identity)
			p.uidToNode.Write(ownerNode)
			p.gc.dirtyQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: p.gc.clock.Now(), Object: ownerNode})
		}
		ownerNode.addDependent(n)
	}
}

// insertNode insert the node to p.uidToNode; then it finds all owners as listed
// in n.owners, and adds the node to their dependents list.
func (p *Propagator) insertNode(n *node) {
	p.uidToNode.Write(n)
	p.addDependentToOwners(n, n.owners)
}

// removeDependentFromOwners remove n from owners' dependents list.
func (p *Propagator) removeDependentFromOwners(n *node, owners []metatypes.OwnerReference) {
	for _, owner := range owners {
		ownerNode, ok := p.uidToNode.Read(owner.UID)
		if !ok {
			continue
		}
		ownerNode.deleteDependent(n)
	}
}

// removeNode removes the node from p.uidToNode, then finds all
// owners as listed in n.owners, and removes n from their dependents list.
func (p *Propagator) removeNode(n *node) {
	p.uidToNode.Delete(n.identity.UID)
	p.removeDependentFromOwners(n, n.owners)
}

// TODO: profile this function to see if a naive N^2 algorithm performs better
// when the number of references is small.
func referencesDiffs(old []metatypes.OwnerReference, new []metatypes.OwnerReference) (added []metatypes.OwnerReference, removed []metatypes.OwnerReference, changed []metatypes.OwnerReference) {
	oldUIDToRef := make(map[string]metatypes.OwnerReference)
	for i := 0; i < len(old); i++ {
		oldUIDToRef[string(old[i].UID)] = old[i]
	}
	oldUIDSet := sets.StringKeySet(oldUIDToRef)
	newUIDToRef := make(map[string]metatypes.OwnerReference)
	for i := 0; i < len(new); i++ {
		newUIDToRef[string(new[i].UID)] = new[i]
	}
	newUIDSet := sets.StringKeySet(newUIDToRef)

	addedUID := newUIDSet.Difference(oldUIDSet)
	removedUID := oldUIDSet.Difference(newUIDSet)
	intersection := oldUIDSet.Intersection(newUIDSet)

	for uid := range addedUID {
		added = append(added, newUIDToRef[uid])
	}
	for uid := range removedUID {
		removed = append(removed, oldUIDToRef[uid])
	}
	for uid := range intersection {
		if !reflect.DeepEqual(oldUIDToRef[uid], newUIDToRef[uid]) {
			changed = append(changed, newUIDToRef[uid])
		}
	}
	return added, removed, changed
}

// returns if the object referred by the event transitions to "being deleted".
func deletionStarts(e *event, accessor meta.Object) bool {
	// The delta_fifo may combine the creation and update of the object into one
	// event, so if there is no e.oldObj, we just return if the e.obj is being deleted.
	if e.oldObj == nil {
		if accessor.GetDeletionTimestamp() == nil {
			return false
		}
	} else {
		oldAccessor, err := meta.Accessor(e.oldObj)
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("cannot access oldObj: %v", err))
			return false
		}
		if err != nil {
			utilruntime.HandleError(fmt.Errorf("cannot access newObj: %v", err))
			return false
		}
		if accessor.GetDeletionTimestamp() == nil || oldAccessor.GetDeletionTimestamp() != nil {
			return false
		}
	}
	return true
}

func beingDeleted(accessor meta.Object) bool {
	return accessor.GetDeletionTimestamp() != nil
}

func hasDeleteDependentsFinalizer(accessor meta.Object) bool {
	finalizers := accessor.GetFinalizers()
	for _, finalizer := range finalizers {
		if finalizer == v1.FinalizerDeleteDependents {
			return true
		}
	}
	return false
}

func startsWaitingForDependentsDeletion(e *event, accessor meta.Object) bool {
	if !deletionStarts(e, accessor) {
		// ignore the event if it's not about the object starting being deleted.
		return false
	}
	return hasDeleteDependentsFinalizer(accessor)
}

func shouldOrphanDependents(e *event, accessor meta.Object) bool {
	if !deletionStarts(e, accessor) {
		// ignore the event if it's not about the object starting being deleted.
		return false
	}
	finalizers := accessor.GetFinalizers()
	for _, finalizer := range finalizers {
		if finalizer == v1.FinalizerOrphanDependents {
			return true
		}
	}
	return false
}

func deleteOwnerRefPatch(dependentUID types.UID, ownerUIDs ...types.UID) []byte {
	var pieces []string
	for _, ownerUID := range ownerUIDs {
		pieces = append(pieces, fmt.Sprintf(`{"$patch":"delete","uid":"%s"}`, ownerUID))
	}
	patch := fmt.Sprintf(`{"metadata":{"ownerReferences":[%s],"uid":"%s"}}`, strings.Join(pieces, ","), dependentUID)
	return []byte(patch)
}

// dependents are copies of pointers to the owner's dependents, they don't need to be locked.
func (gc *GarbageCollector) orhpanDependents(owner objectReference, dependents []*node) error {
	var failedDependents []objectReference
	var errorsSlice []error
	for _, dependent := range dependents {
		// the dependent.identity.UID is used as precondition
		patch := deleteOwnerRefPatch(dependent.identity.UID, owner.UID)
		_, err := gc.patchObject(dependent.identity, patch)
		// note that if the target ownerReference doesn't exist in the
		// dependent, strategic merge patch will NOT return an error.
		if err != nil && !errors.IsNotFound(err) {
			errorsSlice = append(errorsSlice, fmt.Errorf("orphaning %s failed with %v", dependent.identity, err))
		}
	}
	if len(failedDependents) != 0 {
		return fmt.Errorf("failed to orphan dependents of owner %s, got errors: %s", owner, utilerrors.NewAggregate(errorsSlice).Error())
	}
	glog.V(6).Infof("successfully updated all dependents")
	return nil
}

// TODO: Using Patch when strategicmerge supports deleting an entry from a
// slice of a base type.
func (gc *GarbageCollector) removeFinalizer(owner *node, targetFinalizer string) error {
	const retries = 5
	for count := 0; count < retries; count++ {
		ownerObject, err := gc.getObject(owner.identity)
		if err != nil {
			return fmt.Errorf("cannot finalize owner %s, because cannot get it. The garbage collector will retry later.", owner.identity)
		}
		accessor, err := meta.Accessor(ownerObject)
		if err != nil {
			return fmt.Errorf("cannot access the owner object: %v. The garbage collector will retry later.", err)
		}
		finalizers := accessor.GetFinalizers()
		var newFinalizers []string
		found := false
		for _, f := range finalizers {
			if f == targetFinalizer {
				found = true
				break
			} else {
				newFinalizers = append(newFinalizers, f)
			}
		}
		if !found {
			glog.V(6).Infof("the orphan finalizer is already removed from object %s", owner.identity)
			return nil
		}
		// remove the owner from dependent's OwnerReferences
		ownerObject.SetFinalizers(newFinalizers)
		_, err = gc.updateObject(owner.identity, ownerObject)
		if err == nil {
			return nil
		}
		if err != nil && !errors.IsConflict(err) {
			return fmt.Errorf("cannot update the finalizers of owner %s, with error: %v, tried %d times", owner.identity, err, count+1)
		}
		// retry if it's a conflict
		glog.V(6).Infof("got conflict updating the owner object %s, tried %d times", owner.identity, count+1)
	}
	return fmt.Errorf("updateMaxRetries(%d) has reached. The garbage collector will retry later for owner %v.", retries, owner.identity)
}

// orphanFinalizer dequeues a node from the orphanQueue, then finds its dependents
// based on the graph maintained by the GC, then removes it from the
// OwnerReferences of its dependents, and finally updates the owner to remove
// the "Orphan" finalizer. The node is add back into the orphanQueue if any of
// these steps fail.
func (gc *GarbageCollector) orphanFinalizer() {
	timedItem, quit := gc.orphanQueue.Get()
	if quit {
		return
	}
	defer gc.orphanQueue.Done(timedItem)
	owner, ok := timedItem.Object.(*node)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("expect *node, got %#v", timedItem.Object))
	}
	// we don't need to lock each element, because they never get updated
	owner.dependentsLock.RLock()
	dependents := make([]*node, 0, len(owner.dependents))
	for dependent := range owner.dependents {
		dependents = append(dependents, dependent)
	}
	owner.dependentsLock.RUnlock()

	err := gc.orhpanDependents(owner.identity, dependents)
	if err != nil {
		glog.V(6).Infof("orphanDependents for %s failed with %v", owner.identity, err)
		gc.orphanQueue.Add(timedItem)
		return
	}
	// update the owner, remove "orphaningFinalizer" from its finalizers list
	err = gc.removeFinalizer(owner, v1.FinalizerOrphanDependents)
	if err != nil {
		glog.V(6).Infof("removeOrphanFinalizer for %s failed with %v", owner.identity, err)
		gc.orphanQueue.Add(timedItem)
	}
	OrphanProcessingLatency.Observe(sinceInMicroseconds(gc.clock, timedItem.StartTime))
}

// Dequeueing an event from eventQueue, updating graph, populating dirty_queue.
func (p *Propagator) processEvent() {
	timedItem, quit := p.eventQueue.Get()
	if quit {
		return
	}
	defer p.eventQueue.Done(timedItem)
	event, ok := timedItem.Object.(*event)
	if !ok {
		utilruntime.HandleError(fmt.Errorf("expect a *event, got %v", timedItem.Object))
		return
	}
	obj := event.obj
	accessor, err := meta.Accessor(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("cannot access obj: %v", err))
		return
	}
	typeAccessor, err := meta.TypeAccessor(obj)
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("cannot access obj: %v", err))
		return
	}
	glog.V(6).Infof("Propagator process object: %s/%s, namespace %s, name %s, event type %s", typeAccessor.GetAPIVersion(), typeAccessor.GetKind(), accessor.GetNamespace(), accessor.GetName(), event.eventType)
	// Check if the node already exsits
	existingNode, found := p.uidToNode.Read(accessor.GetUID())
	switch {
	case (event.eventType == addEvent || event.eventType == updateEvent) && !found:
		newNode := &node{
			identity: objectReference{
				OwnerReference: metatypes.OwnerReference{
					APIVersion: typeAccessor.GetAPIVersion(),
					Kind:       typeAccessor.GetKind(),
					UID:        accessor.GetUID(),
					Name:       accessor.GetName(),
				},
				Namespace: accessor.GetNamespace(),
			},
			dependents:         make(map[*node]struct{}),
			owners:             accessor.GetOwnerReferences(),
			deletingDependents: beingDeleted(accessor) && hasDeleteDependentsFinalizer(accessor),
			beingDeleted:       beingDeleted(accessor),
		}
		p.insertNode(newNode)
		// the underlying delta_fifo may combine a creation and deletion into one event
		if shouldOrphanDependents(event, accessor) {
			glog.V(6).Infof("add %s to the orphanQueue", newNode.identity)
			p.gc.orphanQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: p.gc.clock.Now(), Object: newNode})
		}
		if startsWaitingForDependentsDeletion(event, accessor) {
			glog.V(2).Infof("add %s to the dirtyQueue, because it's waiting for its dependents to be deleted", newNode.identity)
			p.gc.dirtyQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: p.gc.clock.Now(), Object: newNode})
			for dep := range newNode.dependents {
				p.gc.dirtyQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: p.gc.clock.Now(), Object: dep})
			}
		}
	case (event.eventType == addEvent || event.eventType == updateEvent) && found:
		// caveat: if GC observes the creation of the dependents later than the
		// deletion of the owner, then the orphaning finalizer won't be effective.
		if shouldOrphanDependents(event, accessor) {
			glog.V(6).Infof("add %s to the orphanQueue", existingNode.identity)
			p.gc.orphanQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: p.gc.clock.Now(), Object: existingNode})
		}
		if beingDeleted(accessor) {
			existingNode.beingDeleted = true
		}
		if startsWaitingForDependentsDeletion(event, accessor) {
			glog.V(2).Infof("add %s to the dirtyQueue, because it's waiting for its dependents to be deleted", existingNode.identity)
			// if the existingNode is added as a "virtual" node, its deletingDependents field is not properly set, so always set it here.
			existingNode.deletingDependents = true
			p.gc.dirtyQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: p.gc.clock.Now(), Object: existingNode})
			for dep := range existingNode.dependents {
				p.gc.dirtyQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: p.gc.clock.Now(), Object: dep})
			}
		}
		// add/remove owner refs
		added, removed, changed := referencesDiffs(existingNode.owners, accessor.GetOwnerReferences())
		if len(added) == 0 && len(removed) == 0 && len(changed) == 0 {
			glog.V(6).Infof("The updateEvent %#v doesn't change node references, ignore", event)
			return
		}
		if len(changed) != 0 {
			glog.V(2).Infof("CHAO: references %#v changed for object %s", changed, existingNode.identity)
		}
		// update the node itself
		existingNode.owners = accessor.GetOwnerReferences()
		// Add the node to its new owners' dependent lists.
		p.addDependentToOwners(existingNode, added)
		// remove the node from the dependent list of node that are no long in
		// the node's owners list.
		p.removeDependentFromOwners(existingNode, removed)
	case event.eventType == deleteEvent:
		if !found {
			glog.V(6).Infof("%v doesn't exist in the graph, this shouldn't happen", accessor.GetUID())
			return
		}
		p.removeNode(existingNode)
		existingNode.dependentsLock.RLock()
		defer existingNode.dependentsLock.RUnlock()
		if len(existingNode.dependents) > 0 {
			p.gc.absentOwnerCache.Add(accessor.GetUID())
		}
		for dep := range existingNode.dependents {
			p.gc.dirtyQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: p.gc.clock.Now(), Object: dep})
		}
		for _, owner := range existingNode.owners {
			ownerNode, found := p.uidToNode.Read(owner.UID)
			if !found || !ownerNode.deletingDependents {
				continue
			}
			// this is to let processItem check if all the owner's dependents are deleted.
			p.gc.dirtyQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: p.gc.clock.Now(), Object: ownerNode})
		}
	}
	EventProcessingLatency.Observe(sinceInMicroseconds(p.gc.clock, timedItem.StartTime))
}

// GarbageCollector is responsible for carrying out cascading deletion, and
// removing ownerReferences from the dependents if the owner is deleted with
// DeleteOptions.OrphanDependents=true.
type GarbageCollector struct {
	restMapper meta.RESTMapper
	// metaOnlyClientPool uses a special codec, which removes fields except for
	// apiVersion, kind, and metadata during decoding.
	metaOnlyClientPool dynamic.ClientPool
	// clientPool uses the regular dynamicCodec. We need it to update
	// finalizers. It can be removed if we support patching finalizers.
	clientPool                       dynamic.ClientPool
	dirtyQueue                       *workqueue.TimedWorkQueue
	orphanQueue                      *workqueue.TimedWorkQueue
	monitors                         []monitor
	propagator                       *Propagator
	clock                            clock.Clock
	registeredRateLimiter            *RegisteredRateLimiter
	registeredRateLimiterForMonitors *RegisteredRateLimiter
	// GC caches the owners that do not exist according to the API server.
	absentOwnerCache *UIDCache
}

func gcListWatcher(client *dynamic.Client, resource schema.GroupVersionResource) *cache.ListWatch {
	return &cache.ListWatch{
		ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
			// APIResource.Kind is not used by the dynamic client, so
			// leave it empty. We want to list this resource in all
			// namespaces if it's namespace scoped, so leave
			// APIResource.Namespaced as false is all right.
			apiResource := unversioned.APIResource{Name: resource.Resource}
			return client.ParameterCodec(dynamic.VersionedParameterEncoderWithV1Fallback).
				Resource(&apiResource, v1.NamespaceAll).
				List(&options)
		},
		WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
			// APIResource.Kind is not used by the dynamic client, so
			// leave it empty. We want to list this resource in all
			// namespaces if it's namespace scoped, so leave
			// APIResource.Namespaced as false is all right.
			apiResource := unversioned.APIResource{Name: resource.Resource}
			return client.ParameterCodec(dynamic.VersionedParameterEncoderWithV1Fallback).
				Resource(&apiResource, v1.NamespaceAll).
				Watch(&options)
		},
	}
}

func (gc *GarbageCollector) monitorFor(resource schema.GroupVersionResource, kind schema.GroupVersionKind) (monitor, error) {
	// TODO: consider store in one storage.
	glog.V(6).Infof("create storage for resource %s", resource)
	var monitor monitor
	client, err := gc.metaOnlyClientPool.ClientForGroupVersionKind(kind)
	if err != nil {
		return monitor, err
	}
	gc.registeredRateLimiterForMonitors.registerIfNotPresent(resource.GroupVersion(), client, "garbage_collector_monitoring")
	setObjectTypeMeta := func(obj interface{}) {
		runtimeObject, ok := obj.(runtime.Object)
		if !ok {
			utilruntime.HandleError(fmt.Errorf("expected runtime.Object, got %#v", obj))
		}
		runtimeObject.GetObjectKind().SetGroupVersionKind(kind)
	}
	monitor.store, monitor.controller = cache.NewInformer(
		gcListWatcher(client, resource),
		nil,
		ResourceResyncTime,
		cache.ResourceEventHandlerFuncs{
			// add the event to the propagator's eventQueue.
			AddFunc: func(obj interface{}) {
				setObjectTypeMeta(obj)
				event := &event{
					eventType: addEvent,
					obj:       obj,
				}
				gc.propagator.eventQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: gc.clock.Now(), Object: event})
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				setObjectTypeMeta(newObj)
				event := &event{updateEvent, newObj, oldObj}
				gc.propagator.eventQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: gc.clock.Now(), Object: event})
			},
			DeleteFunc: func(obj interface{}) {
				// delta fifo may wrap the object in a cache.DeletedFinalStateUnknown, unwrap it
				if deletedFinalStateUnknown, ok := obj.(cache.DeletedFinalStateUnknown); ok {
					obj = deletedFinalStateUnknown.Obj
				}
				setObjectTypeMeta(obj)
				event := &event{
					eventType: deleteEvent,
					obj:       obj,
				}
				gc.propagator.eventQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: gc.clock.Now(), Object: event})
			},
		},
	)
	return monitor, nil
}

var ignoredResources = map[schema.GroupVersionResource]struct{}{
	schema.GroupVersionResource{Group: "extensions", Version: "v1beta1", Resource: "replicationcontrollers"}:              {},
	schema.GroupVersionResource{Group: "", Version: "v1", Resource: "bindings"}:                                           {},
	schema.GroupVersionResource{Group: "", Version: "v1", Resource: "componentstatuses"}:                                  {},
	schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}:                                             {},
	schema.GroupVersionResource{Group: "authentication.k8s.io", Version: "v1beta1", Resource: "tokenreviews"}:             {},
	schema.GroupVersionResource{Group: "authorization.k8s.io", Version: "v1beta1", Resource: "subjectaccessreviews"}:      {},
	schema.GroupVersionResource{Group: "authorization.k8s.io", Version: "v1beta1", Resource: "selfsubjectaccessreviews"}:  {},
	schema.GroupVersionResource{Group: "authorization.k8s.io", Version: "v1beta1", Resource: "localsubjectaccessreviews"}: {},
}

func NewGarbageCollector(metaOnlyClientPool dynamic.ClientPool, clientPool dynamic.ClientPool, mapper meta.RESTMapper, resources []schema.GroupVersionResource) (*GarbageCollector, error) {
	gc := &GarbageCollector{
		metaOnlyClientPool:               metaOnlyClientPool,
		clientPool:                       clientPool,
		restMapper:                       mapper,
		clock:                            clock.RealClock{},
		dirtyQueue:                       workqueue.NewTimedWorkQueue(),
		orphanQueue:                      workqueue.NewTimedWorkQueue(),
		registeredRateLimiter:            NewRegisteredRateLimiter(resources),
		registeredRateLimiterForMonitors: NewRegisteredRateLimiter(resources),
		absentOwnerCache:                 NewUIDCache(500),
	}
	gc.propagator = &Propagator{
		eventQueue: workqueue.NewTimedWorkQueue(),
		uidToNode: &concurrentUIDToNode{
			RWMutex:   &sync.RWMutex{},
			uidToNode: make(map[types.UID]*node),
		},
		gc: gc,
	}
	for _, resource := range resources {
		if _, ok := ignoredResources[resource]; ok {
			glog.V(6).Infof("ignore resource %#v", resource)
			continue
		}
		kind, err := gc.restMapper.KindFor(resource)
		if err != nil {
			return nil, err
		}
		monitor, err := gc.monitorFor(resource, kind)
		if err != nil {
			return nil, err
		}
		gc.monitors = append(gc.monitors, monitor)
	}
	return gc, nil
}

func (gc *GarbageCollector) worker() {
	timedItem, quit := gc.dirtyQueue.Get()
	if quit {
		return
	}
	defer gc.dirtyQueue.Done(timedItem)
	err := gc.processItem(timedItem.Object.(*node))
	if err != nil {
		utilruntime.HandleError(fmt.Errorf("Error syncing item %#v: %v", timedItem.Object, err))
		// retry if garbage collection of an object failed.
		gc.dirtyQueue.Add(timedItem)
		return
	}
	DirtyProcessingLatency.Observe(sinceInMicroseconds(gc.clock, timedItem.StartTime))
}

// apiResource consults the REST mapper to translate an <apiVersion, kind,
// namespace> tuple to a unversioned.APIResource struct.
func (gc *GarbageCollector) apiResource(apiVersion, kind string, namespaced bool) (*unversioned.APIResource, error) {
	fqKind := schema.FromAPIVersionAndKind(apiVersion, kind)
	mapping, err := gc.restMapper.RESTMapping(fqKind.GroupKind(), apiVersion)
	if err != nil {
		return nil, fmt.Errorf("unable to get REST mapping for kind: %s, version: %s", kind, apiVersion)
	}
	glog.V(6).Infof("map kind %s, version %s to resource %s", kind, apiVersion, mapping.Resource)
	resource := unversioned.APIResource{
		Name:       mapping.Resource,
		Namespaced: namespaced,
		Kind:       kind,
	}
	return &resource, nil
}

func (gc *GarbageCollector) deleteObject(item objectReference, policy v1.DeletePropagationPolicy) error {
	fqKind := schema.FromAPIVersionAndKind(item.APIVersion, item.Kind)
	client, err := gc.clientPool.ClientForGroupVersionKind(fqKind)
	gc.registeredRateLimiter.registerIfNotPresent(fqKind.GroupVersion(), client, "garbage_collector_operation")
	resource, err := gc.apiResource(item.APIVersion, item.Kind, len(item.Namespace) != 0)
	if err != nil {
		return err
	}
	uid := item.UID
	preconditions := v1.Preconditions{UID: &uid}
	deleteOptions := v1.DeleteOptions{Preconditions: &preconditions, PropagationPolicy: &policy}
	return client.Resource(resource, item.Namespace).Delete(item.Name, &deleteOptions)
}

func (gc *GarbageCollector) getObject(item objectReference) (*runtime.Unstructured, error) {
	fqKind := schema.FromAPIVersionAndKind(item.APIVersion, item.Kind)
	client, err := gc.clientPool.ClientForGroupVersionKind(fqKind)
	gc.registeredRateLimiter.registerIfNotPresent(fqKind.GroupVersion(), client, "garbage_collector_operation")
	resource, err := gc.apiResource(item.APIVersion, item.Kind, len(item.Namespace) != 0)
	if err != nil {
		return nil, err
	}
	return client.Resource(resource, item.Namespace).Get(item.Name)
}

func (gc *GarbageCollector) updateObject(item objectReference, obj *runtime.Unstructured) (*runtime.Unstructured, error) {
	fqKind := schema.FromAPIVersionAndKind(item.APIVersion, item.Kind)
	client, err := gc.clientPool.ClientForGroupVersionKind(fqKind)
	gc.registeredRateLimiter.registerIfNotPresent(fqKind.GroupVersion(), client, "garbage_collector_operation")
	resource, err := gc.apiResource(item.APIVersion, item.Kind, len(item.Namespace) != 0)
	if err != nil {
		return nil, err
	}
	return client.Resource(resource, item.Namespace).Update(obj)
}

func (gc *GarbageCollector) patchObject(item objectReference, patch []byte) (*runtime.Unstructured, error) {
	fqKind := schema.FromAPIVersionAndKind(item.APIVersion, item.Kind)
	client, err := gc.clientPool.ClientForGroupVersionKind(fqKind)
	gc.registeredRateLimiter.registerIfNotPresent(fqKind.GroupVersion(), client, "garbage_collector_operation")
	resource, err := gc.apiResource(item.APIVersion, item.Kind, len(item.Namespace) != 0)
	if err != nil {
		return nil, err
	}
	return client.Resource(resource, item.Namespace).Patch(item.Name, api.StrategicMergePatchType, patch)
}

func objectReferenceToUnstructured(ref objectReference) *runtime.Unstructured {
	ret := &runtime.Unstructured{}
	ret.SetKind(ref.Kind)
	ret.SetAPIVersion(ref.APIVersion)
	ret.SetUID(ref.UID)
	ret.SetNamespace(ref.Namespace)
	ret.SetName(ref.Name)
	return ret
}

func objectReferenceToMetadataOnlyObject(ref objectReference) *metaonly.MetadataOnlyObject {
	return &metaonly.MetadataOnlyObject{
		TypeMeta: unversioned.TypeMeta{
			APIVersion: ref.APIVersion,
			Kind:       ref.Kind,
		},
		ObjectMeta: v1.ObjectMeta{
			Namespace: ref.Namespace,
			UID:       ref.UID,
			Name:      ref.Name,
		},
	}
}

// solid: the owner exists, and is not "waiting"
// dangling: the owner does not exist
// waiting: the owner exists, its deletionTimestamp is non-nil, and it has
// FianlizerDeletingDependents
func (gc *GarbageCollector) classifyReferences(item *node, latestReferences []metatypes.OwnerReference) (
	solid []metatypes.OwnerReference,
	dangling []metatypes.OwnerReference,
	waiting []metatypes.OwnerReference,
	err error,
) {
	for _, reference := range latestReferences {
		if gc.absentOwnerCache.Has(reference.UID) {
			glog.V(6).Infof("according to the absentOwnerCache, object %s's owner %s/%s, %s does not exist", item.identity.UID, reference.APIVersion, reference.Kind, reference.Name)
			dangling = append(dangling, reference)
			continue
		}
		// TODO: we need to verify the reference resource is supported by the
		// system. If it's not a valid resource, the garbage collector should i)
		// ignore the reference when decide if the object should be deleted, and
		// ii) should update the object to remove such references. This is to
		// prevent objects having references to an old resource from being
		// deleted during a cluster upgrade.
		fqKind := schema.FromAPIVersionAndKind(reference.APIVersion, reference.Kind)
		client, err := gc.clientPool.ClientForGroupVersionKind(fqKind)
		if err != nil {
			return solid, dangling, waiting, err
		}
		resource, err := gc.apiResource(reference.APIVersion, reference.Kind, len(item.identity.Namespace) != 0)
		if err != nil {
			return solid, dangling, waiting, err
		}
		owner, err := client.Resource(resource, item.identity.Namespace).Get(reference.Name)
		if err != nil {
			if errors.IsNotFound(err) {
				gc.absentOwnerCache.Add(reference.UID)
				glog.V(6).Infof("object %s's owner %s/%s, %s is not found", item.identity.UID, reference.APIVersion, reference.Kind, reference.Name)
			} else {
				return solid, dangling, waiting, err
			}
		}

		if owner.GetUID() != reference.UID {
			glog.V(6).Infof("object %s's owner %s/%s, %s is not found, UID mismatch", item.identity.UID, reference.APIVersion, reference.Kind, reference.Name)
			gc.absentOwnerCache.Add(reference.UID)
			dangling = append(dangling, reference)
			continue
		}

		ownerAccessor, err := meta.Accessor(owner)
		if err != nil {
			return solid, dangling, waiting, err
		}
		if ownerAccessor.GetDeletionTimestamp() != nil && hasDeleteDependentsFinalizer(ownerAccessor) {
			waiting = append(waiting, reference)
		} else {
			solid = append(solid, reference)
		}
	}
	return solid, dangling, waiting, nil
}

func ownerRefsToUIDs(refs []metatypes.OwnerReference) []types.UID {
	var ret []types.UID
	for _, ref := range refs {
		ret = append(ret, ref.UID)
	}
	return ret
}

func (gc *GarbageCollector) processItem(item *node) error {
	glog.V(2).Infof("CHAO: processing item %s", item.identity)
	// "being deleted" is an one-way trip to the final deletion. We'll just wait for the final deletion, and then process the object's dependents.
	if item.beingDeleted && !item.deletingDependents {
		glog.V(2).Infof("CHAO: processing item %s returned at once", item.identity)
		return nil
	}
	// Get the latest item from the API server
	latest, err := gc.getObject(item.identity)
	if err != nil {
		if errors.IsNotFound(err) {
			// the Propagator can add "virtual" node for an owner that doesn't
			// exist yet, so we need to enqueue a virtual Delete event to remove
			// the virtual node from Propagator.uidToNode.
			glog.V(6).Infof("item %v not found, generating a virtual delete event", item.identity)
			event := &event{
				eventType: deleteEvent,
				obj:       objectReferenceToMetadataOnlyObject(item.identity),
			}
			glog.V(6).Infof("generating virtual delete event for %s\n\n", event.obj)
			gc.propagator.eventQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: gc.clock.Now(), Object: event})
			return nil
		}
		return err
	}
	if latest.GetUID() != item.identity.UID {
		glog.V(6).Infof("UID doesn't match, item %v not found, generating a virtual delete event", item.identity)
		event := &event{
			eventType: deleteEvent,
			obj:       objectReferenceToMetadataOnlyObject(item.identity),
		}
		glog.V(6).Infof("generating virtual delete event for %s\n\n", event.obj)
		gc.propagator.eventQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: gc.clock.Now(), Object: event})
		return nil
	}

	// TODO: orphanFinalizer() routine is similar. Consider merging orphanFinalizer() into processItem() as well.
	if item.deletingDependents {
		blockingDependents := item.blockingDependents()
		if len(blockingDependents) != 0 {
			glog.V(2).Infof("CHAO: remove DeleteDependents finalizer for item %s", item.identity)
			return gc.removeFinalizer(item, v1.FinalizerDeleteDependents)
		} else {
			for _, dep := range blockingDependents {
				glog.V(2).Infof("CHAO: adding dep %s to dirtyQueue, because %s is deletingDependents", dep.identity, item.identity)
				// TODO: perhaps should only enqueue if the dependent is not
				// already in the deleting state (imagining item is a deployment,
				// and dep is a rs)
				gc.dirtyQueue.Add(&workqueue.TimedWorkQueueItem{StartTime: gc.clock.Now(), Object: dep})
			}
			return nil
		}
	}

	ownerReferences := latest.GetOwnerReferences()
	if len(ownerReferences) == 0 {
		glog.V(2).Infof("CHAO: object %s's doesn't have an owner, continue on next item", item.identity)
		return nil
	}
	solid, dangling, waiting, err := gc.classifyReferences(item, ownerReferences)
	if err != nil {
		return err
	}
	if len(solid) != 0 {
		glog.V(2).Infof("CHAO: object %s has at least an existing owner, will not garbage collect", item.identity)
		if len(dangling) != 0 || len(waiting) != 0 {
			glog.V(2).Infof("CHAO: remove dangling references %#v and waiting references %#v for object %s", dangling, waiting, item.identity)
		}
		patch := deleteOwnerRefPatch(item.identity.UID, append(ownerRefsToUIDs(dangling), ownerRefsToUIDs(waiting)...)...)
		_, err = gc.patchObject(item.identity, patch)
		return err
	} else {
		if len(waiting) != 0 && len(item.dependents) != 0 {
			for dep := range item.dependents {
				if dep.deletingDependents {
					glog.V(2).Infof("processing object %s, some of its owners and its dependent [%s] have FianlizerDeletingDependents, to prevent potential cycle, its ownerReferences are going to be modified to be non-blocking, then the object is going to be deleted with DeletePropagationForeground, and ", item.identity, dep.identity)
					patch, err := item.unblockOwnerReferences()
					if err != nil {
						return err
					}
					if _, err := gc.patchObject(item.identity, patch); err != nil {
						return err
					}
					break
				}
			}
			glog.V(2).Infof("CHAO: at least one owner of object %s has FianlizerDeletingDependents, and the object itself has dependents, so it is going to be deleted with DeletePropagationForeground", item.identity)
			return gc.deleteObject(item.identity, v1.DeletePropagationForeground)
		}
		glog.V(2).Infof("CHAO: delete object %s with DeletePropagationDefault", item.identity)
		return gc.deleteObject(item.identity, v1.DeletePropagationDefault)
	}
}

func (gc *GarbageCollector) Run(workers int, stopCh <-chan struct{}) {
	glog.Infof("Garbage Collector: Initializing")
	for _, monitor := range gc.monitors {
		go monitor.controller.Run(stopCh)
	}

	wait.PollInfinite(10*time.Second, func() (bool, error) {
		for _, monitor := range gc.monitors {
			if !monitor.controller.HasSynced() {
				glog.Infof("Garbage Collector: Waiting for resource monitors to be synced...")
				return false, nil
			}
		}
		return true, nil
	})
	glog.Infof("Garbage Collector: All monitored resources synced. Proceeding to collect garbage")

	// worker
	go wait.Until(gc.propagator.processEvent, 0, stopCh)

	for i := 0; i < workers; i++ {
		go wait.Until(gc.worker, 0, stopCh)
		go wait.Until(gc.orphanFinalizer, 0, stopCh)
	}
	Register()
	<-stopCh
	glog.Infof("Garbage Collector: Shutting down")
	gc.dirtyQueue.ShutDown()
	gc.orphanQueue.ShutDown()
	gc.propagator.eventQueue.ShutDown()
}

// *FOR TEST USE ONLY* It's not safe to call this function when the GC is still
// busy.
// GraphHasUID returns if the Propagator has a particular UID store in its
// uidToNode graph. It's useful for debugging.
func (gc *GarbageCollector) GraphHasUID(UIDs []types.UID) bool {
	for _, u := range UIDs {
		if _, ok := gc.propagator.uidToNode.Read(u); ok {
			return true
		}
	}
	return false
}
