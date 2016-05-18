/*
Copyright 2014 The Kubernetes Authors All rights reserved.

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

package registry

import (
	"fmt"
	"reflect"
	"sync"

	"k8s.io/kubernetes/pkg/api"
	kubeerr "k8s.io/kubernetes/pkg/api/errors"
	storeerr "k8s.io/kubernetes/pkg/api/errors/storage"
	"k8s.io/kubernetes/pkg/api/meta"
	"k8s.io/kubernetes/pkg/api/rest"
	"k8s.io/kubernetes/pkg/api/unversioned"
	"k8s.io/kubernetes/pkg/api/validation"
	"k8s.io/kubernetes/pkg/controller/garbagecollector"
	"k8s.io/kubernetes/pkg/fields"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/registry/generic"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/storage"
	utilruntime "k8s.io/kubernetes/pkg/util/runtime"
	"k8s.io/kubernetes/pkg/util/validation/field"
	"k8s.io/kubernetes/pkg/watch"

	"github.com/golang/glog"
)

// Store implements generic.Registry.
// It's intended to be embeddable, so that you can implement any
// non-generic functions if needed.
// You must supply a value for every field below before use; these are
// left public as it's meant to be overridable if need be.
// This object is intended to be copyable so that it can be used in
// different ways but share the same underlying behavior.
//
// The intended use of this type is embedding within a Kind specific
// RESTStorage implementation. This type provides CRUD semantics on
// a Kubelike resource, handling details like conflict detection with
// ResourceVersion and semantics. The RESTCreateStrategy and
// RESTUpdateStrategy are generic across all backends, and encapsulate
// logic specific to the API.
//
// TODO: make the default exposed methods exactly match a generic RESTStorage
type Store struct {
	// Called to make a new object, should return e.g., &api.Pod{}
	NewFunc func() runtime.Object

	// Called to make a new listing object, should return e.g., &api.PodList{}
	NewListFunc func() runtime.Object

	// Used for error reporting
	QualifiedResource unversioned.GroupResource

	// Used for listing/watching; should not include trailing "/"
	KeyRootFunc func(ctx api.Context) string

	// Called for Create/Update/Get/Delete. Note that 'namespace' can be
	// gotten from ctx.
	KeyFunc func(ctx api.Context, name string) (string, error)

	// Called to get the name of an object
	ObjectNameFunc func(obj runtime.Object) (string, error)

	// Return the TTL objects should be persisted with. Update is true if this
	// is an operation against an existing object. Existing is the current TTL
	// or the default for this operation.
	TTLFunc func(obj runtime.Object, existing uint64, update bool) (uint64, error)

	// Returns a matcher corresponding to the provided labels and fields.
	PredicateFunc func(label labels.Selector, field fields.Selector) generic.Matcher

	// DeleteCollectionWorkers is the maximum number of workers in a single
	// DeleteCollection call.
	DeleteCollectionWorkers int

	// Called on all objects returned from the underlying store, after
	// the exit hooks are invoked. Decorators are intended for integrations
	// that are above storage and should only be used for specific cases where
	// storage of the value is not appropriate, since they cannot
	// be watched.
	Decorator rest.ObjectFunc
	// Allows extended behavior during creation, required
	CreateStrategy rest.RESTCreateStrategy
	// On create of an object, attempt to run a further operation.
	AfterCreate rest.ObjectFunc
	// Allows extended behavior during updates, required
	UpdateStrategy rest.RESTUpdateStrategy
	// On update of an object, attempt to run a further operation.
	AfterUpdate rest.ObjectFunc
	// Allows extended behavior during updates, optional
	DeleteStrategy rest.RESTDeleteStrategy
	// On deletion of an object, attempt to run a further operation.
	AfterDelete rest.ObjectFunc
	// If true, return the object that was deleted. Otherwise, return a generic
	// success status response.
	ReturnDeletedObject bool
	// Allows extended behavior during export, optional
	ExportStrategy rest.RESTExportStrategy

	// Used for all storage access functions
	Storage storage.Interface
}

// NamespaceKeyRootFunc is the default function for constructing storage paths to resource directories enforcing namespace rules.
func NamespaceKeyRootFunc(ctx api.Context, prefix string) string {
	key := prefix
	ns, ok := api.NamespaceFrom(ctx)
	if ok && len(ns) > 0 {
		key = key + "/" + ns
	}
	return key
}

// NamespaceKeyFunc is the default function for constructing storage paths to a resource relative to prefix enforcing namespace rules.
// If no namespace is on context, it errors.
func NamespaceKeyFunc(ctx api.Context, prefix string, name string) (string, error) {
	key := NamespaceKeyRootFunc(ctx, prefix)
	ns, ok := api.NamespaceFrom(ctx)
	if !ok || len(ns) == 0 {
		return "", kubeerr.NewBadRequest("Namespace parameter required.")
	}
	if len(name) == 0 {
		return "", kubeerr.NewBadRequest("Name parameter required.")
	}
	if ok, msg := validation.IsValidPathSegmentName(name); !ok {
		return "", kubeerr.NewBadRequest(fmt.Sprintf("Name parameter invalid: %v.", msg))
	}
	key = key + "/" + name
	return key, nil
}

// NoNamespaceKeyFunc is the default function for constructing storage paths to a resource relative to prefix without a namespace
func NoNamespaceKeyFunc(ctx api.Context, prefix string, name string) (string, error) {
	if len(name) == 0 {
		return "", kubeerr.NewBadRequest("Name parameter required.")
	}
	if ok, msg := validation.IsValidPathSegmentName(name); !ok {
		return "", kubeerr.NewBadRequest(fmt.Sprintf("Name parameter invalid: %v.", msg))
	}
	key := prefix + "/" + name
	return key, nil
}

// New implements RESTStorage
func (e *Store) New() runtime.Object {
	return e.NewFunc()
}

// NewList implements RESTLister
func (e *Store) NewList() runtime.Object {
	return e.NewListFunc()
}

// List returns a list of items matching labels and field
func (e *Store) List(ctx api.Context, options *api.ListOptions) (runtime.Object, error) {
	label := labels.Everything()
	if options != nil && options.LabelSelector != nil {
		label = options.LabelSelector
	}
	field := fields.Everything()
	if options != nil && options.FieldSelector != nil {
		field = options.FieldSelector
	}
	return e.ListPredicate(ctx, e.PredicateFunc(label, field), options)
}

// ListPredicate returns a list of all the items matching m.
func (e *Store) ListPredicate(ctx api.Context, m generic.Matcher, options *api.ListOptions) (runtime.Object, error) {
	list := e.NewListFunc()
	filterFunc := e.filterAndDecorateFunction(m)
	if name, ok := m.MatchesSingle(); ok {
		if key, err := e.KeyFunc(ctx, name); err == nil {
			err := e.Storage.GetToList(ctx, key, filterFunc, list)
			return list, storeerr.InterpretListError(err, e.QualifiedResource)
		}
		// if we cannot extract a key based on the current context, the optimization is skipped
	}

	if options == nil {
		options = &api.ListOptions{ResourceVersion: "0"}
	}
	err := e.Storage.List(ctx, e.KeyRootFunc(ctx), options.ResourceVersion, filterFunc, list)
	return list, storeerr.InterpretListError(err, e.QualifiedResource)
}

// Create inserts a new item according to the unique key from the object.
func (e *Store) Create(ctx api.Context, obj runtime.Object) (runtime.Object, error) {
	if err := rest.BeforeCreate(e.CreateStrategy, ctx, obj); err != nil {
		return nil, err
	}
	name, err := e.ObjectNameFunc(obj)
	if err != nil {
		return nil, err
	}
	key, err := e.KeyFunc(ctx, name)
	if err != nil {
		return nil, err
	}
	ttl, err := e.calculateTTL(obj, 0, false)
	if err != nil {
		return nil, err
	}
	out := e.NewFunc()
	if err := e.Storage.Create(ctx, key, obj, out, ttl); err != nil {
		err = storeerr.InterpretCreateError(err, e.QualifiedResource, name)
		err = rest.CheckGeneratedNameError(e.CreateStrategy, err, obj)
		return nil, err
	}
	if e.AfterCreate != nil {
		if err := e.AfterCreate(out); err != nil {
			return nil, err
		}
	}
	if e.Decorator != nil {
		if err := e.Decorator(obj); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// shouldDelete checks if a Update is removing all the object's finalizers. If so,
// it further checks if the object's DeletionGracePeriodSeconds is 0. If so, it
// returns true.
func (e *Store) shouldDelete(ctx api.Context, key string, obj, existing runtime.Object) bool {
	if err := e.Storage.Get(ctx, key, existing, false); err != nil {
		utilruntime.HandleError(err)
		return false
	}
	// Note: we do not check doUnconditionalUpdate in this routine, because we
	// expect in most cases the update of finalizers will be made by non-human,
	// and we expect the resourceVersion will be properly set.

	// validate the update.
	if err := rest.BeforeUpdate(e.UpdateStrategy, ctx, obj, existing); err != nil {
		utilruntime.HandleError(err)
		return false
	}
	newMeta, err := api.ObjectMetaFor(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return false
	}
	oldMeta, err := api.ObjectMetaFor(existing)
	if err != nil {
		utilruntime.HandleError(err)
		return false
	}
	return len(oldMeta.Finalizers) != 0 && len(newMeta.Finalizers) == 0 && newMeta.DeletionGracePeriodSeconds != nil && *newMeta.DeletionGracePeriodSeconds == 0
}

// Update performs an atomic update and set of the object. Returns the result of the update
// or an error. If the registry allows create-on-update, the create flow will be executed.
// A bool is returned along with the object and any errors, to indicate object creation.
func (e *Store) Update(ctx api.Context, obj runtime.Object) (runtime.Object, bool, error) {
	name, err := e.ObjectNameFunc(obj)
	if err != nil {
		return nil, false, err
	}
	key, err := e.KeyFunc(ctx, name)
	if err != nil {
		return nil, false, err
	}
	// If AllowUnconditionalUpdate() is true and the object specified by the user does not have a resource version,
	// then we populate it with the latest version.
	// Else, we check that the version specified by the user matches the version of latest storage object.
	resourceVersion, err := e.Storage.Versioner().ObjectResourceVersion(obj)
	if err != nil {
		return nil, false, err
	}
	doUnconditionalUpdate := resourceVersion == 0 && e.UpdateStrategy.AllowUnconditionalUpdate()
	// TODO: expose TTL
	creating := false
	out := e.NewFunc()
	meta, err := api.ObjectMetaFor(obj)
	if err != nil {
		return nil, false, kubeerr.NewInternalError(err)
	}
	var preconditions *storage.Preconditions
	// If the UID of the new object is specified, we use it as an Update precondition.
	if len(meta.UID) != 0 {
		UIDCopy := meta.UID
		preconditions = &storage.Preconditions{UID: &UIDCopy}
	}
	// check if the update is removing all the object's finalizers. If so,
	// further checks if the object's DeletionGracePeriodSeconds is 0. If so,
	// delete the object.
	existing := e.NewFunc()
	delete := e.shouldDelete(ctx, key, obj, existing)
	if delete {
		out := e.NewFunc()
		glog.V(6).Infof("going to delete %s from regitry, triggered by update", name)
		if err := e.Storage.Delete(ctx, key, out, preconditions); err != nil {
			// Deletion is racy, i.e., there could be multiple update
			// requests to remove all finalizers from the object, so we
			// ignore the NotFound error.
			if storage.IsNotFound(err) {
				ret, err := e.finalizeDelete(existing, true)
				return ret, false, err
			}
			return nil, false, storeerr.InterpretDeleteError(err, e.QualifiedResource, name)
		}
		ret, err := e.finalizeDelete(out, true)
		return ret, false, err
	}

	err = e.Storage.GuaranteedUpdate(ctx, key, out, true, preconditions, func(existing runtime.Object, res storage.ResponseMeta) (runtime.Object, *uint64, error) {
		// Since we return 'obj' from this function and it can be modified outside this
		// function, we are resetting resourceVersion to the initial value here.
		//
		// TODO: In fact, we should probably return a DeepCopy of obj in all places.
		err := e.Storage.Versioner().UpdateObject(obj, resourceVersion)
		if err != nil {
			return nil, nil, err
		}

		version, err := e.Storage.Versioner().ObjectResourceVersion(existing)
		if err != nil {
			return nil, nil, err
		}
		if version == 0 {
			if !e.UpdateStrategy.AllowCreateOnUpdate() {
				return nil, nil, kubeerr.NewNotFound(e.QualifiedResource, name)
			}
			creating = true
			if err := rest.BeforeCreate(e.CreateStrategy, ctx, obj); err != nil {
				return nil, nil, err
			}
			ttl, err := e.calculateTTL(obj, 0, false)
			if err != nil {
				return nil, nil, err
			}
			return obj, &ttl, nil
		}

		creating = false
		if doUnconditionalUpdate {
			// Update the object's resource version to match the latest storage object's resource version.
			err = e.Storage.Versioner().UpdateObject(obj, res.ResourceVersion)
			if err != nil {
				return nil, nil, err
			}
		} else {
			// Check if the object's resource version matches the latest resource version.
			newVersion, err := e.Storage.Versioner().ObjectResourceVersion(obj)
			if err != nil {
				return nil, nil, err
			}
			if newVersion == 0 {
				// TODO: The Invalid error should has a field for Resource.
				// After that field is added, we should fill the Resource and
				// leave the Kind field empty. See the discussion in #18526.
				qualifiedKind := unversioned.GroupKind{Group: e.QualifiedResource.Group, Kind: e.QualifiedResource.Resource}
				fieldErrList := field.ErrorList{field.Invalid(field.NewPath("metadata").Child("resourceVersion"), newVersion, "must be specified for an update")}
				return nil, nil, kubeerr.NewInvalid(qualifiedKind, name, fieldErrList)
			}
			if newVersion != version {
				return nil, nil, kubeerr.NewConflict(e.QualifiedResource, name, fmt.Errorf("the object has been modified; please apply your changes to the latest version and try again"))
			}
		}
		if err := rest.BeforeUpdate(e.UpdateStrategy, ctx, obj, existing); err != nil {
			return nil, nil, err
		}
		ttl, err := e.calculateTTL(obj, res.TTL, true)
		if err != nil {
			return nil, nil, err
		}
		if int64(ttl) != res.TTL {
			return obj, &ttl, nil
		}
		return obj, nil, nil
	})

	if err != nil {
		if creating {
			err = storeerr.InterpretCreateError(err, e.QualifiedResource, name)
			err = rest.CheckGeneratedNameError(e.CreateStrategy, err, obj)
		} else {
			err = storeerr.InterpretUpdateError(err, e.QualifiedResource, name)
		}
		return nil, false, err
	}
	if creating {
		if e.AfterCreate != nil {
			if err := e.AfterCreate(out); err != nil {
				return nil, false, err
			}
		}
	} else {
		if e.AfterUpdate != nil {
			if err := e.AfterUpdate(out); err != nil {
				return nil, false, err
			}
		}
	}
	if e.Decorator != nil {
		if err := e.Decorator(obj); err != nil {
			return nil, false, err
		}
	}
	return out, creating, nil
}

// Get retrieves the item from storage.
func (e *Store) Get(ctx api.Context, name string) (runtime.Object, error) {
	obj := e.NewFunc()
	key, err := e.KeyFunc(ctx, name)
	if err != nil {
		return nil, err
	}
	if err := e.Storage.Get(ctx, key, obj, false); err != nil {
		return nil, storeerr.InterpretGetError(err, e.QualifiedResource, name)
	}
	if e.Decorator != nil {
		if err := e.Decorator(obj); err != nil {
			return nil, err
		}
	}
	return obj, nil
}

var (
	errAlreadyDeleting = fmt.Errorf("abort delete")
	errDeleteNow       = fmt.Errorf("delete now")
)

// return if we need to update the finalizers of the object
func shouldUpdateFinalizers(accessor meta.Object, options *api.DeleteOptions) bool {
	shouldOrphan := options != nil && options.OrphanDependents != nil && *options.OrphanDependents
	orphanSet := false
	for _, f := range accessor.GetFinalizers() {
		if f == garbagecollector.OrphanFinalizerID {
			orphanSet = true
		}
	}
	return shouldOrphan != orphanSet
}

// add/remove the finalizers as the `options` requires
func updateFinalizers(accessor meta.Object, options *api.DeleteOptions) {
	shouldOrphan := options != nil && options.OrphanDependents != nil && *options.OrphanDependents
	alreadyOrphan := false
	finalizers := accessor.GetFinalizers()
	newFinalizers := make([]string, 0, len(finalizers))
	for _, f := range finalizers {
		if f == garbagecollector.OrphanFinalizerID {
			alreadyOrphan = true
			if !shouldOrphan {
				continue
			}
		}
		newFinalizers = append(newFinalizers, f)
	}
	if shouldOrphan && !alreadyOrphan {
		newFinalizers = append(newFinalizers, garbagecollector.OrphanFinalizerID)
	}
	accessor.SetFinalizers(newFinalizers)
}

// Delete removes the item from storage.
func (e *Store) Delete(ctx api.Context, name string, options *api.DeleteOptions) (runtime.Object, error) {
	key, err := e.KeyFunc(ctx, name)
	if err != nil {
		return nil, err
	}

	obj := e.NewFunc()
	if err := e.Storage.Get(ctx, key, obj, false); err != nil {
		return nil, storeerr.InterpretDeleteError(err, e.QualifiedResource, name)
	}
	// support older consumers of delete by treating "nil" as delete immediately
	if options == nil {
		options = api.NewDeleteOptions(0)
	}
	var preconditions storage.Preconditions
	if options.Preconditions != nil {
		preconditions.UID = options.Preconditions.UID
	}
	graceful, pendingGraceful, err := rest.BeforeDelete(e.DeleteStrategy, ctx, obj, options)
	if err != nil {
		return nil, err
	}
	// this means finalizers cannot be updated via DeleteOptions if a deletion is already pending
	if pendingGraceful {
		return e.finalizeDelete(obj, false)
	}
	// check if obj has pending finalizers
	accessor, err := meta.Accessor(obj)
	if err != nil {
		return nil, kubeerr.NewInternalError(err)
	}
	pendingFinalizers := len(accessor.GetFinalizers()) != 0
	var ignoreNotFound bool = false
	var lastExisting runtime.Object = nil
	// update the object
	if graceful || pendingFinalizers || shouldUpdateFinalizers(accessor, options) {
		out := e.NewFunc()
		lastGraceful := int64(0)
		err := e.Storage.GuaranteedUpdate(
			ctx, key, out, false, &preconditions,
			storage.SimpleUpdate(func(existing runtime.Object) (runtime.Object, error) {
				graceful, pendingGraceful, err := rest.BeforeDelete(e.DeleteStrategy, ctx, existing, options)
				if err != nil {
					return nil, err
				}
				if pendingGraceful {
					return nil, errAlreadyDeleting
				}

				// add/remove the orphan finalizer as the options dictates
				existingAccessor, err := meta.Accessor(existing)
				if err != nil {
					return nil, err
				}
				updateFinalizers(existingAccessor, options)

				if !graceful {
					// set the DeleteGracePeriods to 0 if the object has pendingFinalizers but not supporting graceful deletion
					pendingFinalizers = len(existingAccessor.GetFinalizers()) != 0
					if pendingFinalizers {
						glog.V(6).Infof("update the DeletionTimestamp to \"now\" and GracePeriodSeconds to 0 for object %s, because it has pending finalizers", name)
						err = rest.MarkImmediateDeletion(e.DeleteStrategy, existing)
						if err != nil {
							return nil, err
						}
						return existing, nil
					}
					return nil, errDeleteNow
				}
				lastGraceful = *options.GracePeriodSeconds
				lastExisting = existing
				return existing, nil
			}),
		)
		switch err {
		case nil:
			// if the object has pending finalizers, don't delete it, no matter
			// whether the deletion is graceful, and no matter if the grace
			// period is 0.
			if pendingFinalizers {
				return out, nil
			}
			if lastGraceful > 0 {
				return out, nil
			}
			// If we are here, the registry supports grace period mechanism and
			// we are intentionally delete gracelessly. In this case, we may
			// enter a race with other k8s components. If other component wins
			// the race, the object will not be found, and we should tolerate
			// the NotFound error. See
			// https://github.com/kubernetes/kubernetes/issues/19403 for
			// details.
			ignoreNotFound = true
			// exit the switch and delete immediately
		case errDeleteNow:
			// we've updated the object to have a zero grace period, or it's already at 0, so
			// we should fall through and truly delete the object.
		case errAlreadyDeleting:
			return e.finalizeDelete(obj, true)
		default:
			return nil, storeerr.InterpretUpdateError(err, e.QualifiedResource, name)
		}
	}

	// delete immediately, or no graceful deletion supported
	out := e.NewFunc()
	glog.V(6).Infof("going to delete %s from regitry: ", name)
	if err := e.Storage.Delete(ctx, key, out, &preconditions); err != nil {
		// Please refer to the place where we set ignoreNotFound for the reason
		// why we ignore the NotFound error .
		if storage.IsNotFound(err) && ignoreNotFound && lastExisting != nil {
			// The lastExisting object may not be the last state of the object
			// before its deletion, but it's the best approximation.
			return e.finalizeDelete(lastExisting, true)
		}
		return nil, storeerr.InterpretDeleteError(err, e.QualifiedResource, name)
	}
	return e.finalizeDelete(out, true)
}

// DeleteCollection remove all items returned by List with a given ListOptions from storage.
//
// DeleteCollection is currently NOT atomic. It can happen that only subset of objects
// will be deleted from storage, and then an error will be returned.
// In case of success, the list of deleted objects will be returned.
//
// TODO: Currently, there is no easy way to remove 'directory' entry from storage (if we
// are removing all objects of a given type) with the current API (it's technically
// possibly with storage API, but watch is not delivered correctly then).
// It will be possible to fix it with v3 etcd API.
func (e *Store) DeleteCollection(ctx api.Context, options *api.DeleteOptions, listOptions *api.ListOptions) (runtime.Object, error) {
	listObj, err := e.List(ctx, listOptions)
	if err != nil {
		return nil, err
	}
	items, err := meta.ExtractList(listObj)
	if err != nil {
		return nil, err
	}
	// Spawn a number of goroutines, so that we can issue requests to storage
	// in parallel to speed up deletion.
	// TODO: Make this proportional to the number of items to delete, up to
	// DeleteCollectionWorkers (it doesn't make much sense to spawn 16
	// workers to delete 10 items).
	workersNumber := e.DeleteCollectionWorkers
	if workersNumber < 1 {
		workersNumber = 1
	}
	wg := sync.WaitGroup{}
	toProcess := make(chan int, 2*workersNumber)
	errs := make(chan error, workersNumber+1)

	go func() {
		defer utilruntime.HandleCrash(func(panicReason interface{}) {
			errs <- fmt.Errorf("DeleteCollection distributor panicked: %v", panicReason)
		})
		for i := 0; i < len(items); i++ {
			toProcess <- i
		}
		close(toProcess)
	}()

	wg.Add(workersNumber)
	for i := 0; i < workersNumber; i++ {
		go func() {
			// panics don't cross goroutine boundaries
			defer utilruntime.HandleCrash(func(panicReason interface{}) {
				errs <- fmt.Errorf("DeleteCollection goroutine panicked: %v", panicReason)
			})
			defer wg.Done()

			for {
				index, ok := <-toProcess
				if !ok {
					return
				}
				accessor, err := meta.Accessor(items[index])
				if err != nil {
					errs <- err
					return
				}
				if _, err := e.Delete(ctx, accessor.GetName(), options); err != nil && !kubeerr.IsNotFound(err) {
					glog.V(4).Infof("Delete %s in DeleteCollection failed: %v", accessor.GetName(), err)
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errs:
		return nil, err
	default:
		return listObj, nil
	}
}

func (e *Store) finalizeDelete(obj runtime.Object, runHooks bool) (runtime.Object, error) {
	if runHooks && e.AfterDelete != nil {
		if err := e.AfterDelete(obj); err != nil {
			return nil, err
		}
	}
	if e.ReturnDeletedObject {
		if e.Decorator != nil {
			if err := e.Decorator(obj); err != nil {
				return nil, err
			}
		}
		return obj, nil
	}
	return &unversioned.Status{Status: unversioned.StatusSuccess}, nil
}

// Watch makes a matcher for the given label and field, and calls
// WatchPredicate. If possible, you should customize PredicateFunc to produre a
// matcher that matches by key. generic.SelectionPredicate does this for you
// automatically.
func (e *Store) Watch(ctx api.Context, options *api.ListOptions) (watch.Interface, error) {
	label := labels.Everything()
	if options != nil && options.LabelSelector != nil {
		label = options.LabelSelector
	}
	field := fields.Everything()
	if options != nil && options.FieldSelector != nil {
		field = options.FieldSelector
	}
	resourceVersion := ""
	if options != nil {
		resourceVersion = options.ResourceVersion
	}
	return e.WatchPredicate(ctx, e.PredicateFunc(label, field), resourceVersion)
}

// WatchPredicate starts a watch for the items that m matches.
func (e *Store) WatchPredicate(ctx api.Context, m generic.Matcher, resourceVersion string) (watch.Interface, error) {
	filterFunc := e.filterAndDecorateFunction(m)

	if name, ok := m.MatchesSingle(); ok {
		if key, err := e.KeyFunc(ctx, name); err == nil {
			if err != nil {
				return nil, err
			}
			return e.Storage.Watch(ctx, key, resourceVersion, filterFunc)
		}
		// if we cannot extract a key based on the current context, the optimization is skipped
	}

	return e.Storage.WatchList(ctx, e.KeyRootFunc(ctx), resourceVersion, filterFunc)
}

func (e *Store) filterAndDecorateFunction(m generic.Matcher) func(runtime.Object) bool {
	return func(obj runtime.Object) bool {
		matches, err := m.Matches(obj)
		if err != nil {
			glog.Errorf("unable to match watch: %v", err)
			return false
		}
		if matches && e.Decorator != nil {
			if err := e.Decorator(obj); err != nil {
				glog.Errorf("unable to decorate watch: %v", err)
				return false
			}
		}
		return matches
	}
}

// calculateTTL is a helper for retrieving the updated TTL for an object or returning an error
// if the TTL cannot be calculated. The defaultTTL is changed to 1 if less than zero. Zero means
// no TTL, not expire immediately.
func (e *Store) calculateTTL(obj runtime.Object, defaultTTL int64, update bool) (ttl uint64, err error) {
	// TODO: validate this is assertion is still valid.
	// etcd may return a negative TTL for a node if the expiration has not occurred due
	// to server lag - we will ensure that the value is at least set.
	if defaultTTL < 0 {
		defaultTTL = 1
	}
	ttl = uint64(defaultTTL)
	if e.TTLFunc != nil {
		ttl, err = e.TTLFunc(obj, ttl, update)
	}
	return ttl, err
}

func exportObjectMeta(accessor meta.Object, exact bool) {
	accessor.SetUID("")
	if !exact {
		accessor.SetNamespace("")
	}
	accessor.SetCreationTimestamp(unversioned.Time{})
	accessor.SetDeletionTimestamp(nil)
	accessor.SetResourceVersion("")
	accessor.SetSelfLink("")
	if len(accessor.GetGenerateName()) > 0 && !exact {
		accessor.SetName("")
	}
}

// Implements the rest.Exporter interface
func (e *Store) Export(ctx api.Context, name string, opts unversioned.ExportOptions) (runtime.Object, error) {
	obj, err := e.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if accessor, err := meta.Accessor(obj); err == nil {
		exportObjectMeta(accessor, opts.Exact)
	} else {
		glog.V(4).Infof("Object of type %v does not have ObjectMeta: %v", reflect.TypeOf(obj), err)
	}

	if e.ExportStrategy != nil {
		if err = e.ExportStrategy.Export(obj, opts.Exact); err != nil {
			return nil, err
		}
	} else {
		e.CreateStrategy.PrepareForCreate(obj)
	}
	return obj, nil
}
