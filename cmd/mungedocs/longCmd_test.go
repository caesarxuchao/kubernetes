/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestRemoveStringSpace(t *testing.T) {
	var cases = []struct {
		in  string
		out string
	}{
		{"", ""},
		{
			"I'm a \"double quote\"",
			"I'm a \"double\x00quote\"",
		},
		{
			"Im a \"double quote ' contains a 'single quote\"",
			"Im a \"double\x00quote\x00'\x00contains\x00a\x00'single\x00quote\"",
		},
		{
			"\"I'm a quote\", \"I'm a quote\"",
			"\"I'm\x00a\x00quote\", \"I'm\x00a\x00quote\"",
		},
		{
			"Im \\\"escaped, so space\"",
			"Im \\\"escaped, so space\"",
		},
	}
	for i, c := range cases {
		actual := removeStringSpace(c.in)
		if string(actual) != c.out {
			t.Errorf("case[%d]: expected %q got %q", i, c.out, string(actual))
		}
	}
}

func TestBreakLine(t *testing.T) {
	var cases = []struct {
		in  string
		out []string
	}{
		{"", []string{""}},
		{
			strings.Repeat("a", 80),
			[]string{strings.Repeat("a", 80)},
		},
		{
			strings.Repeat("a", 80) + "b",
			nil,
		},
		{
			"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt",
			[]string{
				"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor \\",
				"  incididunt",
			},
		},
		{
			"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod \"tempor incididunt\"",
			[]string{
				"Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod \\",
				"  \"tempor incididunt\"",
			},
		},
		{
			"Lorem\" ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt\"",
			nil,
		},
	}
	for i, c := range cases {
		actual, _ := breakLine(c.in)
		if !reflect.DeepEqual(actual, c.out) {
			t.Errorf("case[%d]: expected %q got %q", i, c.out, actual)
		}
	}
}
