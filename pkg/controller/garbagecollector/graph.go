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
	"fmt"
	"sync"

	"k8s.io/kubernetes/pkg/api/meta/metatypes"
	"k8s.io/kubernetes/pkg/types"
)

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

// blockingDependents returns the dependents that are blocking the deletion of
// n.
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
