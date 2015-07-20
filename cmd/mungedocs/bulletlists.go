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
	"strings"
)

const (
	bullet = iota
	whitespace
)

var bulletRegex = regexp.MustCompile(`(^\s*\*.\s*.*$)|(^\s*-.\s*.*$)|(^\s*[\d]{1,2}\.\s*.*$)`)
var leadingWhitespaceRegex = regexp.MustCompile(`^\s*`)

func countLeadingWhitespace(line string) int {
	return len(leadingWhitespaceRegex.FindString(line))
}

func findPrevBulletOrSpace(out []string) (int, int) {
	for i := len(out) - 1; i >= 0; i-- {
		if whitespaceRegex.MatchString(out[i]) {
			return whitespace, i
		}
		if bulletRegex.MatchString(out[i]) {
			return bullet, i
		}
	}
	//treat start of file block as a whitespace
	return whitespace, -1
}

func fixBulletLists(fileBytes []byte) []byte {
	lines := splitLines(fileBytes)
	out := []string{}
	for i := range lines {
		if !bulletRegex.MatchString(lines[i]) {
			out = append(out, lines[i])
			continue
		}
		fmt.Println("CHAO: find match.", lines[i])

		pre, j := findPrevBulletOrSpace(out)
		switch pre {
		case whitespace:
			// if there is another block of text before the list with out a whitespace in between
			if j != len(out)-1 {
				out = append(out, "")
			}
		case bullet:
			space1 := countLeadingWhitespace(out[j])
			space2 := countLeadingWhitespace(lines[i])
			//nested list
			if space1 < space2 {
				out = append(out, "")
			}
		}
		out = append(out, lines[i])
	}
	final := strings.Join(out, "\n")
	// Preserve the end of the file.
	if len(fileBytes) > 0 && fileBytes[len(fileBytes)-1] == '\n' {
		final += "\n"
	}
	return []byte(final)
}

// add a blank line before bullet lists is there is none.
func checkBulletLists(filePath string, fileBytes []byte) ([]byte, error) {
	fbs := splitByPreformatted(fileBytes)
	fbs = append([]fileBlock{{false, []byte{}}}, fbs...)
	fbs = append(fbs, fileBlock{false, []byte{}})

	for i := range fbs {
		block := &fbs[i]
		if block.preformatted {
			continue
		}
		block.data = fixBulletLists(block.data)
	}
	output := []byte{}
	for _, block := range fbs {
		output = append(output, block.data...)
	}
	return output, nil
}

//func main() {
//err := filepath.Walk("/usr/local/google/home/xuchao/go-workspace/src/github.com/GoogleCloudPlatform/kubernetes" , newWalkFunc(&fp, &changesNeeded))
//	if err != nil {
//		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
//		os.Exit(2)
//	}
//}
