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

package util

import "strings"

type GroupVersion struct {
	// Group represents the name of the group
	Group string
	// LatestGroupVersion represents the current external default version of
	// the group. It is in the form of "group/version".
	LatestGroupVersion string

	// LatestVersion represents the current external default version of the
	// group. It equals to the "version" part of DefaultGroupVersion
	LatestVersion string

	// OldestVersion represents the oldest server version supported, for client
	// code that wants to hardcode the lowest common denominator.
	OldestVersion string

	// GroupVersions is the list of "group/versions" that are recognized in
	// code.  The order provided may be assumed to be least feature rich to most
	// feature rich, and clients may choose to prefer the latter items in the
	// list over the former items when presented with a set of versions to
	// choose.
	GroupVersions []string

	// Versions is the "version" part of GroupVersions
	Versions []string
}

func (g *GroupVersion) Init(registeredGroupVersions []string) {
	// Use the first registered GroupVersion as the latest.
	g.LatestGroupVersion = registeredGroupVersions[0]
	g.Group = GetGroup(g.LatestGroupVersion)
	g.LatestVersion = GetVersion(g.LatestGroupVersion)
	g.OldestVersion = registeredGroupVersions[len(registeredGroupVersions)-1]
	// Put the registered groupVersions in GroupVersions in reverse order.
	for i := len(registeredGroupVersions) - 1; i >= 0; i-- {
		g.GroupVersions = append(g.GroupVersions, registeredGroupVersions[i])
		g.Versions = append(g.Versions, GetVersion(registeredGroupVersions[i]))
	}
}

func GetVersion(groupVersion string) string {
	s := strings.Split(groupVersion, "/")
	if len(s) != 2 {
		//e.g. return "v1" for groupVersion="v1"
		return s[len(s)-1]
	}
	return s[1]
}

func GetGroup(groupVersion string) string {
	s := strings.Split(groupVersion, "/")
	if len(s) == 1 {
		//e.g. return "" for groupVersion="v1"
		return ""
	}
	return s[0]
}
