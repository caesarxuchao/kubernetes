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

package cmd

import (
	"io"

	"github.com/spf13/cobra"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/kubectl"
	cmdutil "github.com/GoogleCloudPlatform/kubernetes/pkg/kubectl/cmd/util"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util"
)

//const (
//	replace_long = `Replace a resource by filename or stdin.
//
//JSON and YAML formats are accepted.`
//	replace_example = `// Replace a pod using the data in pod.json.
//$ kubectl replace -f pod.json
//
//// Replace a pod based on the JSON passed into stdin.
//$ cat pod.json | kubectl replace -f -
//
//// Force replace, delete and then re-create the resource
//kubectl replace --force -f pod.json`
//)

func NewCmdUpdate(f *cmdutil.Factory, out io.Writer) *cobra.Command {
	var filenames util.StringList
	cmd := &cobra.Command{
		name:       "update",
		Deprecated: "update is DEPRECATED, please use replace instead",
		Run: func(cmd *cobra.Command, args []string) {
			err := RunReplace(f, out, cmd, args, filenames)
			cmdutil.CheckCustomErr("Replace failed", err)
		},
	}
	usage := "Filename, directory, or URL to file to use to replace the resource."
	kubectl.AddJsonFilenameFlag(cmd, &filenames, usage)
	cmd.MarkFlagRequired("filename")
	cmd.Flags().Bool("force", false, "Delete and re-create the specified resource")
	cmd.Flags().Bool("cascade", false, "Only relevant during a force replace. If true, cascade the deletion of the resources managed by this resource (e.g. Pods created by a ReplicationController).  Default true.")
	cmd.Flags().Int("grace-period", -1, "Only relevant during a force replace. Period of time in seconds given to the old resource to terminate gracefully. Ignored if negative.")
	cmd.Flags().Duration("timeout", 0, "Only relevant during a force replace. The length of time to wait before giving up on a delete of the old resource, zero means determine a timeout from the size of the object")
	return cmd
}
