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
	"fmt"
	"regexp"
)

// Blocks of ``` need to have blank lines on both sides or they don't look
// right in HTML.

var cmdRegex = regexp.MustCompile(`^\s*(\$.*$)`)

func checkLongCmd(filePath string, fileBytes []byte) ([]byte, error) {
	f := splitByPreformatted(fileBytes)
	f = append(fileBlocks{{false, []byte{}}}, f...)
	f = append(f, fileBlock{false, []byte{}})

	longCmd := false
	for i := 1; i < len(f)-1; i++ {
		block := &f[i]

		if !block.preformatted {
			continue
		}
		lines := splitLines(block.data)
		for i := range lines {
			if len(lines[i]) > 80 {
				matches := cmdRegex.FindStringSubmatch(lines[i])
				if matches == nil {
					continue
				}
				if len(matches[0]) > 80 {
					fmt.Println(lines[i])
					longCmd = true
				}
			}
		}
	}
	if longCmd {
		return fileBytes, fmt.Errorf("")
	} else {
		return fileBytes, nil
	}
}
