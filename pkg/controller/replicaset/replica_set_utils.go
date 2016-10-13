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

// If you make changes to this file, you should also make the corresponding change in ReplicationController.

package replicaset

import (
	"fmt"

	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/apis/extensions"
	unversionedextensions "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset/typed/extensions/unversioned"
)

// updateReplicaCount attempts to update the Status.Replicas of the given ReplicaSet, with a single GET/PUT retry.
func updateReplicaCount(rsClient unversionedextensions.ReplicaSetInterface, rs extensions.ReplicaSet, numReplicas, numFullyLabeledReplicas, numReadyReplicas, numAvailableReplicas int) (updateErr error) {
	// This is the steady state. It happens when the ReplicaSet doesn't have any expectations, since
	// we do a periodic relist every 30s. If the generations differ but the replicas are
	// the same, a caller might've resized to the same replica count.
	if int(rs.Status.Replicas) == numReplicas &&
		int(rs.Status.FullyLabeledReplicas) == numFullyLabeledReplicas &&
		int(rs.Status.ReadyReplicas) == numReadyReplicas &&
		int(rs.Status.AvailableReplicas) == numAvailableReplicas &&
		rs.Generation == rs.Status.ObservedGeneration {
		return nil
	}
	// Save the generation number we acted on, otherwise we might wrongfully indicate
	// that we've seen a spec update when we retry.
	// TODO: This can clobber an update if we allow multiple agents to write to the
	// same status.
	generation := rs.Generation

	var getErr error
	for i, rs := 0, &rs; ; i++ {
		glog.V(4).Infof(fmt.Sprintf("Updating replica count for ReplicaSet: %s/%s, ", rs.Namespace, rs.Name) +
			fmt.Sprintf("replicas %d->%d (need %d), ", rs.Status.Replicas, numReplicas, rs.Spec.Replicas) +
			fmt.Sprintf("fullyLabeledReplicas %d->%d, ", rs.Status.FullyLabeledReplicas, numFullyLabeledReplicas) +
			fmt.Sprintf("readyReplicas %d->%d, ", rs.Status.ReadyReplicas, numReadyReplicas) +
			fmt.Sprintf("availableReplicas %d->%d, ", rs.Status.AvailableReplicas, numAvailableReplicas) +
			fmt.Sprintf("sequence No: %v->%v", rs.Status.ObservedGeneration, generation))

		rs.Status = extensions.ReplicaSetStatus{
			Replicas:             int32(numReplicas),
			FullyLabeledReplicas: int32(numFullyLabeledReplicas),
			ReadyReplicas:        int32(numReadyReplicas),
			AvailableReplicas:    int32(numAvailableReplicas),
			ObservedGeneration:   generation,
		}
		_, updateErr = rsClient.UpdateStatus(rs)
		if updateErr == nil || i >= statusUpdateRetries {
			return updateErr
		}
		// Update the ReplicaSet with the latest resource version for the next poll
		if rs, getErr = rsClient.Get(rs.Name); getErr != nil {
			// If the GET fails we can't trust status.Replicas anymore. This error
			// is bound to be more interesting than the update failure.
			return getErr
		}
	}
}

// overlappingReplicaSets sorts a list of ReplicaSets by creation timestamp, using their names as a tie breaker.
type overlappingReplicaSets []*extensions.ReplicaSet

func (o overlappingReplicaSets) Len() int      { return len(o) }
func (o overlappingReplicaSets) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o overlappingReplicaSets) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(o[j].CreationTimestamp)
}
