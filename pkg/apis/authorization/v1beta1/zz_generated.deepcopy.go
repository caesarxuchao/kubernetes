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

package v1beta1

import (
	reflect "reflect"

	"k8s.io/api/authorization/v1beta1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1beta1_LocalSubjectAccessReview, InType: reflect.TypeOf(&v1beta1.LocalSubjectAccessReview{})},
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1beta1_NonResourceAttributes, InType: reflect.TypeOf(&v1beta1.NonResourceAttributes{})},
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1beta1_ResourceAttributes, InType: reflect.TypeOf(&v1beta1.ResourceAttributes{})},
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1beta1_SelfSubjectAccessReview, InType: reflect.TypeOf(&v1beta1.SelfSubjectAccessReview{})},
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1beta1_SelfSubjectAccessReviewSpec, InType: reflect.TypeOf(&v1beta1.SelfSubjectAccessReviewSpec{})},
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1beta1_SubjectAccessReview, InType: reflect.TypeOf(&v1beta1.SubjectAccessReview{})},
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1beta1_SubjectAccessReviewSpec, InType: reflect.TypeOf(&v1beta1.SubjectAccessReviewSpec{})},
		conversion.GeneratedDeepCopyFunc{Fn: DeepCopy_v1beta1_SubjectAccessReviewStatus, InType: reflect.TypeOf(&v1beta1.SubjectAccessReviewStatus{})},
	)
}

func DeepCopy_v1beta1_LocalSubjectAccessReview(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1beta1.LocalSubjectAccessReview)
		out := out.(*v1beta1.LocalSubjectAccessReview)
		*out = *in
		if newVal, err := c.DeepCopy(&in.ObjectMeta); err != nil {
			return err
		} else {
			out.ObjectMeta = *newVal.(*v1.ObjectMeta)
		}
		if err := DeepCopy_v1beta1_SubjectAccessReviewSpec(&in.Spec, &out.Spec, c); err != nil {
			return err
		}
		return nil
	}
}

func DeepCopy_v1beta1_NonResourceAttributes(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1beta1.NonResourceAttributes)
		out := out.(*v1beta1.NonResourceAttributes)
		*out = *in
		return nil
	}
}

func DeepCopy_v1beta1_ResourceAttributes(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1beta1.ResourceAttributes)
		out := out.(*v1beta1.ResourceAttributes)
		*out = *in
		return nil
	}
}

func DeepCopy_v1beta1_SelfSubjectAccessReview(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1beta1.SelfSubjectAccessReview)
		out := out.(*v1beta1.SelfSubjectAccessReview)
		*out = *in
		if newVal, err := c.DeepCopy(&in.ObjectMeta); err != nil {
			return err
		} else {
			out.ObjectMeta = *newVal.(*v1.ObjectMeta)
		}
		if err := DeepCopy_v1beta1_SelfSubjectAccessReviewSpec(&in.Spec, &out.Spec, c); err != nil {
			return err
		}
		return nil
	}
}

func DeepCopy_v1beta1_SelfSubjectAccessReviewSpec(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1beta1.SelfSubjectAccessReviewSpec)
		out := out.(*v1beta1.SelfSubjectAccessReviewSpec)
		*out = *in
		if in.ResourceAttributes != nil {
			in, out := &in.ResourceAttributes, &out.ResourceAttributes
			*out = new(v1beta1.ResourceAttributes)
			**out = **in
		}
		if in.NonResourceAttributes != nil {
			in, out := &in.NonResourceAttributes, &out.NonResourceAttributes
			*out = new(v1beta1.NonResourceAttributes)
			**out = **in
		}
		return nil
	}
}

func DeepCopy_v1beta1_SubjectAccessReview(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1beta1.SubjectAccessReview)
		out := out.(*v1beta1.SubjectAccessReview)
		*out = *in
		if newVal, err := c.DeepCopy(&in.ObjectMeta); err != nil {
			return err
		} else {
			out.ObjectMeta = *newVal.(*v1.ObjectMeta)
		}
		if err := DeepCopy_v1beta1_SubjectAccessReviewSpec(&in.Spec, &out.Spec, c); err != nil {
			return err
		}
		return nil
	}
}

func DeepCopy_v1beta1_SubjectAccessReviewSpec(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1beta1.SubjectAccessReviewSpec)
		out := out.(*v1beta1.SubjectAccessReviewSpec)
		*out = *in
		if in.ResourceAttributes != nil {
			in, out := &in.ResourceAttributes, &out.ResourceAttributes
			*out = new(v1beta1.ResourceAttributes)
			**out = **in
		}
		if in.NonResourceAttributes != nil {
			in, out := &in.NonResourceAttributes, &out.NonResourceAttributes
			*out = new(v1beta1.NonResourceAttributes)
			**out = **in
		}
		if in.Groups != nil {
			in, out := &in.Groups, &out.Groups
			*out = make([]string, len(*in))
			copy(*out, *in)
		}
		if in.Extra != nil {
			in, out := &in.Extra, &out.Extra
			*out = make(map[string]v1beta1.ExtraValue)
			for key, val := range *in {
				if newVal, err := c.DeepCopy(&val); err != nil {
					return err
				} else {
					(*out)[key] = *newVal.(*v1beta1.ExtraValue)
				}
			}
		}
		return nil
	}
}

func DeepCopy_v1beta1_SubjectAccessReviewStatus(in interface{}, out interface{}, c *conversion.Cloner) error {
	{
		in := in.(*v1beta1.SubjectAccessReviewStatus)
		out := out.(*v1beta1.SubjectAccessReviewStatus)
		*out = *in
		return nil
	}
}
