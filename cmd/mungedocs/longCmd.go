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

//split long commands in docs/ and examples
package main

import (
	"fmt"
	"regexp"
	"strings"
)

var cmdRE = regexp.MustCompile(`^\s*(\$.*$)`)
var spaceRE = regexp.MustCompile(`\s`)
var replacementRE = regexp.MustCompile(`\x00`)

//remove the space in quotes, because we don't want to split inside a quote
func removeStringSpace(in string) string {
	out := in
	firstSingleQuote := -1
	firstDoubleQuote := -1
	escape := false
	for i, r := range in {
		if escape == true {
			escape = false
		} else if string(r) == "\\" {
			escape = true
		} else if string(r) == "'" {
			if firstSingleQuote == -1 {
				firstSingleQuote = i
			} else {
				out = out[:firstSingleQuote] + spaceRE.ReplaceAllString(in[firstSingleQuote:i], "\x00") + out[i:]
				if firstDoubleQuote > firstSingleQuote {
					firstDoubleQuote = -1
				}
				firstSingleQuote = -1
			}
		} else if string(r) == "\"" {
			if firstDoubleQuote == -1 {
				firstDoubleQuote = i
			} else {
				out = out[:firstDoubleQuote] + spaceRE.ReplaceAllString(in[firstDoubleQuote:i], "\x00") + out[i:]
				if firstSingleQuote > firstDoubleQuote {
					firstSingleQuote = -1
				}
				firstDoubleQuote = -1
			}
		}
	}
	return out
}

func addSpaceBack(in []string) {
	for i, line := range in {
		in[i] = replacementRE.ReplaceAllString(line, " ")
	}
}

func breakLine(in string) ([]string, error) {
	in = removeStringSpace(in)
	out := []string{}
	secondHalf := in
	firstLine := true
	for len(secondHalf) > 80 {
		firstHalf := secondHalf[:80]
		i := strings.LastIndex(firstHalf, " ")
		if i == -1 {
			out = append(out, secondHalf)
			return nil, fmt.Errorf("Unable to automatically break line: %q", in)
		}
		if firstLine {
			out = append(out, secondHalf[:i+1]+"\\")
			firstLine = false
		} else {
			out = append(out, "  "+secondHalf[:i+1]+"\\")
		}
		secondHalf = secondHalf[i+1:]
	}
	if firstLine {
		out = append(out, secondHalf)
	} else {
		out = append(out, "  "+secondHalf)
	}
	addSpaceBack(out)
	return out, nil
}

func printSlice(in []string) {
	fmt.Println("Split to:")
	for _, line := range in {
		fmt.Println(line)
	}
}

func fixLongCmd(blockBytes []byte) ([]byte, error) {
	out := []string{}
	lines := splitLines(blockBytes)
	for _, line := range lines {
		matches := cmdRE.FindStringSubmatch(line)
		if matches != nil && len(matches[1]) > 80 {
			if !*Verify {
				fmt.Printf("\nTrying to split command: %q\n", line)
			}
			brokenLines, err := breakLine(line)
			if err != nil {
				out = append(out, line)
				if !*Verify {
					fmt.Println(err)
				}
			} else {
				if !*Verify {
					printSlice(brokenLines)
				}
				out = append(out, brokenLines...)
			}
		} else {
			out = append(out, line)
		}
	}
	final := strings.Join(out, "\n")
	// Preserve the end of the file.
	if len(blockBytes) > 0 && blockBytes[len(blockBytes)-1] == '\n' {
		final += "\n"
	}
	return []byte(final), nil
}

func checkLongCmd(filePath string, fileBytes []byte) ([]byte, error) {
	fbs := splitByPreformatted(fileBytes)
	for i := range fbs {
		block := &fbs[i]
		if !block.preformatted {
			continue
		}
		block.data, _ = fixLongCmd(block.data)
	}
	output := []byte{}
	for _, block := range fbs {
		output = append(output, block.data...)
	}
	return output, nil
}
