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

// Package webhook delegates admission checks to dynamically configured webhooks.
package mutating

import (
	admissionv1alpha1 "k8s.io/api/admission/v1alpha1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/admission"
)

// createAdmissionReview creates an AdmissionReview for the provided admission.Attributes
func createAdmissionReview(convertor runtime.ObjectConvertor, attr admission.Attributes, ir *intermidiateAdmissionResult) (admissionv1alpha1.AdmissionReview, error) {
	gvk := attr.GetKind()
	gvr := attr.GetResource()
	aUserInfo := attr.GetUserInfo()
	userInfo := authenticationv1.UserInfo{
		Extra:    make(map[string]authenticationv1.ExtraValue),
		Groups:   aUserInfo.GetGroups(),
		UID:      aUserInfo.GetUID(),
		Username: aUserInfo.GetName(),
	}

	// Convert the extra information in the user object
	for key, val := range aUserInfo.GetExtra() {
		userInfo.Extra[key] = authenticationv1.ExtraValue(val)
	}

	var err error
	if ir.versionedOldObject == nil && attr.GetOldObject() != nil {
		ir.versionedOldObject, err = convertor.ConvertToVersion(attr.GetOldObject(), schema.GroupVersion{Group: gvr.Group, Version: gvr.Version})
		if err != nil {
			return admissionv1alpha1.AdmissionReview{}, err
		}
	}
	if ir.versionedObject == nil && attr.GetObject() != nil {
		ir.versionedObject, err = convertor.ConvertToVersion(attr.GetObject(), schema.GroupVersion{Group: gvr.Group, Version: gvr.Version})
		if err != nil {
			return admissionv1alpha1.AdmissionReview{}, err
		}
	}

	return admissionv1alpha1.AdmissionReview{
		Spec: admissionv1alpha1.AdmissionReviewSpec{
			Name:      attr.GetName(),
			Namespace: attr.GetNamespace(),
			Resource: metav1.GroupVersionResource{
				Group:    gvr.Group,
				Resource: gvr.Resource,
				Version:  gvr.Version,
			},
			SubResource: attr.GetSubresource(),
			Operation:   admissionv1alpha1.Operation(attr.GetOperation()),
			Object: runtime.RawExtension{
				Object: ir.versionedObject,
				// Note that Raw takes precedence over Object when serialized.
				Raw: ir.mutatedObjRaw,
			},
			OldObject: runtime.RawExtension{
				Object: ir.versionedOldObject,
			},
			Kind: metav1.GroupVersionKind{
				Group:   gvk.Group,
				Kind:    gvk.Kind,
				Version: gvk.Version,
			},
			UserInfo: userInfo,
		},
	}, nil
}
