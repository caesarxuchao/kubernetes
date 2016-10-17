#!/bin/bash
# convert pkg/controller/ to use client-go


# PART I: convert client imports
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset" "k8s.io/client-go/1.5/kubernetes"
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/client/cache" "k8s.io/client-go/1.5/tools/cache"
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/client/record" "k8s.io/client-go/1.5/tools/record"
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/client/typed/dynamic" "k8s.io/client-go/1.5/dynamic"
# TODO
$K1/grep-sed.sh "\"k8s.io/kubernetes/pkg/client/testing/core\"" "core \"k8s.io/client-go/1.5/testing\""
$K1/grep-sed.sh "\"k8s.io/kubernetes/pkg/client/restclient\"" "restclient \"k8s.io/client-go/1.5/rest\""

# PART I.1: corner cases
$K1/grep-sed.sh "k8s.io/client-go/1.5/kubernetes/typed/core/unversioned" "k8s.io/client-go/1.5/kubernetes/typed/core/v1"
$K1/grep-sed.sh "unversionedcore" "v1core"
#TODO:
$K1/grep-sed.sh "k8s.io/client-go/1.5/kubernetes/typed/extensions/unversioned" "k8s.io/client-go/1.5/kubernetes/typed/extensions/v1beta1"
$K1/grep-sed.sh "k8s.io/client-go/1.5/kubernetes/typed/policy/unversioned" "k8s.io/client-go/1.5/kubernetes/typed/policy/v1alpha1"
$K1/grep-sed.sh "k8s.io/client-go/1.5/kubernetes/typed/apps/unversioned" "k8s.io/client-go/1.5/kubernetes/typed/apps/v1alpha1"
$K1/grep-sed.sh "k8s.io/client-go/1.5/kubernetes/typed/autoscaling/unversioned" "k8s.io/client-go/1.5/kubernetes/typed/autoscaling/v1"

# PART II: convert type imports
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/api\"" "k8s.io/client-go/1.5/pkg/api\"\n\"k8s.io/client-go/1.5/pkg/api/v1\""
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/api" "k8s.io/client-go/1.5/pkg/api"
# don't convert "k8s.io/kubernetes/pkg/api/validation"
$K1/grep-sed.sh "k8s.io/client-go/1.5/pkg/api/validation" "k8s.io/kubernetes/pkg/api/validation" 
# don't convert "k8s.io/kubernetes/pkg/api/annotations"
$K1/grep-sed.sh "k8s.io/client-go/1.5/pkg/api/annotations" "k8s.io/kubernetes/pkg/api/annotations"
# don't convert "k8s.io/kubernetes/pkg/api/endpoints", need a copy of it that deals with client-go types.
$K1/grep-sed.sh "k8s.io/client-go/1.5/pkg/api/endpoints" "k8s.io/kubernetes/pkg/api/endpoints" 
# don't convert "k8s.io/kubernetes/pkg/api/pod", need a copy of it that deals with client-go types.
$K1/grep-sed.sh "k8s.io/client-go/1.5/pkg/api/pod" "k8s.io/kubernetes/pkg/api/pod"

$K1/grep-sed.sh "k8s.io/kubernetes/pkg/apis" "k8s.io/client-go/1.5/pkg/apis"

# PART III: rewrite api. to v1.
$K1/grep-sed.sh "api\." "v1."

# Don't rewrite metrics_api to metrics_v1
$K1/grep-sed.sh "metrics_v1" "metrics_api"


# PART IV: dependencies of client-go/kubernetes,
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/watch" "k8s.io/client-go/1.5/pkg/watch"
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/runtime" "k8s.io/client-go/1.5/pkg/runtime"
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/labels" "k8s.io/client-go/1.5/pkg/labels"
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/fields" "k8s.io/client-go/1.5/pkg/fields"
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/types" "k8s.io/client-go/1.5/pkg/types"

#PART V: use volumeshadow
$K1/grep-sed.sh "k8s.io/kubernetes/pkg/volume" "k8s.io/kubernetes/pkg/volumeshadow"
set -x
find ./ -type f -name "*.go" | grep -v volume/persistentvolume/index.go | grep -v volume/persistentvolume/pv_controller.go | grep -v volume/persistentvolume/pv_controller_base.go | xargs sed -i "s,volume\.,volumeshadow\.,g"
set +x

#pkg/watch
#pkg/runtime

# Use the pkg/api/field_constants.go, or copy it to somewhere in client-go?

# api.CreatedByAnnotation, not v1
# api.StrategicMergePatchType, not v1






#volume.NewSpecFromPersistentVolume, it's in pkg/volume, it's taking k8s.io/kubernetes types.
# in general, what to do with calls to pkg/volume?

# copy GetAccessModesAsString from pkg/api/helpers.go to pkg/v1/helpers.go

# NOTES
find ./ -name "*.go" | xargs gofmt -w

goimports -w ./
