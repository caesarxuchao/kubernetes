// +build !ignore_autogenerated

/*
Copyright 2017 The Kubernetes Authors.

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

// This file was autogenerated by deepcopy-gen. Do not edit it manually!

package v1

import (
	reflect "reflect"

	"k8s.io/api/storage/v1"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	conversion "k8s.io/apimachinery/pkg/conversion"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

func init() {
	SchemeBuilder.Register(RegisterDeepCopies)
}

// RegisterDeepCopies adds deep-copy functions to the given scheme. Public
// to allow building arbitrary schemes.
func RegisterDeepCopies(scheme *runtime.Scheme) error {
	return scheme.AddGeneratedDeepCopyFuncs(
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1_StorageClass, InType: reflect.TypeOf(&v1.StorageClass{})},
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1_StorageClassList, InType: reflect.TypeOf(&v1.StorageClassList{})},
	)
}

func DeepCopy_v1_StorageClass(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1.StorageClass)
		out := out.(*v1.StorageClass)
		*out = *in
		if newVal, err := c.DeepCopy(&in.ObjectMeta); err != nil {
			return err
		} else {
			out.ObjectMeta = *newVal.(*meta_v1.ObjectMeta)
		}
		if in.Parameters != nil {
			in, out := &in.Parameters, &out.Parameters
			*out = make(map[string]string)
			for key, val := range *in {
				(*out)[key] = val
			}
		}
		return nil
	}
}

func DeepCopy_v1_StorageClassList(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1.StorageClassList)
		out := out.(*v1.StorageClassList)
		*out = *in
		if in.Items != nil {
			in, out := &in.Items, &out.Items
			*out = make([]v1.StorageClass, len(*in))
			for i := range *in {
				if err := DeepCopy_v1_StorageClass(&(*in)[i], &(*out)[i], c); err != nil {
					return err
				}
			}
		}
		return nil
	}
}
