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

package generators

import (
	"io"

	"k8s.io/kubernetes/cmd/libs/go2idl/generator"
	"k8s.io/kubernetes/cmd/libs/go2idl/namer"
	"k8s.io/kubernetes/cmd/libs/go2idl/types"
)

// genGroup produces a file for a group client, e.g. ExtensionsClient for the extension group.
type genGroup struct {
	generator.DefaultGen
	outputPackage string
	group         string
	// types in this group
	types   []*types.Type
	imports *generator.ImportTracker
}

// We only want to call GenerateType() once per group.
func (g *genGroup) Filter(c *generator.Context, t *types.Type) bool {
	return t == g.types[0]
}

func (g *genGroup) Namers(c *generator.Context) namer.NameSystems {
	return namer.NameSystems{
		"raw": namer.NewRawNamer(g.outputPackage, g.imports),
	}
}

func (g *genGroup) Imports(c *generator.Context) (imports []string) {
	return append(g.imports.ImportLines(), "fmt")
}

func (g *genGroup) GenerateType(c *generator.Context, t *types.Type, w io.Writer) error {
	sw := generator.NewSnippetWriter(w, c, "$", "$")
	const pkgUnversioned = "k8s.io/kubernetes/pkg/client/unversioned"
	const pkgLatest = "k8s.io/kubernetes/pkg/api/latest"
	prefix := func(group string) string {
		if group == "legacy" {
			return `"/api"`
		}
		return `"/apis"`
	}

	canonize := func(group string) string {
		if group == "legacy" {
			return ""
		}
		return group
	}

	m := map[string]interface{}{
		"group":                      g.group,
		"Group":                      namer.IC(g.group),
		"canonicalGroup":             canonize(g.group),
		"types":                      g.types,
		"Config":                     c.Universe.Type(types.Name{Package: pkgUnversioned, Name: "Config"}),
		"DefaultKubernetesUserAgent": c.Universe.Function(types.Name{Package: pkgUnversioned, Name: "DefaultKubernetesUserAgent"}),
		"RESTClient":                 c.Universe.Type(types.Name{Package: pkgUnversioned, Name: "RESTClient"}),
		"RESTClientFor":              c.Universe.Function(types.Name{Package: pkgUnversioned, Name: "RESTClientFor"}),
		"latestGroup":                c.Universe.Variable(types.Name{Package: pkgLatest, Name: "Group"}),
		"GroupOrDie":                 c.Universe.Variable(types.Name{Package: pkgLatest, Name: "GroupOrDie"}),
		"prefix":                     prefix(g.group),
	}
	sw.Do(groupInterfaceTemplate, m)
	sw.Do(groupClientTemplate, m)
	for _, t := range g.types {
		wrapper := map[string]interface{}{
			"type":  t,
			"Group": namer.IC(g.group),
		}
		sw.Do(namespacerImplTemplate, wrapper)
	}
	sw.Do(newClientForConfigTemplate, m)
	sw.Do(newClientForConfigOrDieTemplate, m)
	sw.Do(newClientForRESTClientTemplate, m)
	sw.Do(setClientDefaultsTemplate, m)

	return sw.Error()
}

var groupInterfaceTemplate = `
type $.Group$Interface interface {
    $range .types$ $.Name.Name$Namespacer
    $end$
}
`

var groupClientTemplate = `
// $.Group$Client is used to interact with features provided by the $.Group$ group.
type $.Group$Client struct {
	*$.RESTClient|raw$
}
`

var namespacerImplTemplate = `
func (c *$.Group$Client) $.type|publicPlural$(namespace string) $.type.Name.Name$Interface {
	return new$.type|publicPlural$(c, namespace)
}
`

var newClientForConfigTemplate = `
// New$.Group$ creates a new $.Group$Client for the given config.
func New$.Group$ForConfig(c *$.Config|raw$) (*$.Group$Client, error) {
	config := *c
	if err := set$.Group$Defaults(&config); err != nil {
		return nil, err
	}
	client, err := $.RESTClientFor|raw$(&config)
	if err != nil {
		return nil, err
	}
	return &$.Group$Client{client}, nil
}
`

var newClientForConfigOrDieTemplate = `
// New$.Group$OrDie creates a new $.Group$Client for the given config and
// panics if there is an error in the config.
func New$.Group$ForConfigOrDie(c *$.Config|raw$) *$.Group$Client {
	client, err := New$.Group$ForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}
`

var newClientForRESTClientTemplate = `
// New$.Group$ creates a new $.Group$Client for the given RESTClient.
func New$.Group$ForRESTClient(c *$.RESTClient|raw$) *$.Group$Client {
	return &$.Group$Client{c}
}
`
var setClientDefaultsTemplate = `
func set$.Group$Defaults(config *$.Config|raw$) error {
	// if $.group$ group is not registered, return an error
	g, err := $.latestGroup|raw$("$.canonicalGroup$")
	if err != nil {
		return err
	}
	config.Prefix = $.prefix$
	if config.UserAgent == "" {
		config.UserAgent = $.DefaultKubernetesUserAgent|raw$()
	}
	// TODO: Unconditionally set the config.Version, until we fix the config.
	//if config.Version == "" {
	copyGroupVersion := g.GroupVersion
	config.GroupVersion = &copyGroupVersion
	//}

	versionInterfaces, err := g.InterfacesFor(*config.GroupVersion)
	if err != nil {
		return fmt.Errorf("$.Group$ API version '%s' is not recognized (valid values: %s)",
			config.GroupVersion, g.GroupVersions)
	}
	config.Codec = versionInterfaces.Codec
	if config.QPS == 0 {
		config.QPS = 5
	}
	if config.Burst == 0 {
		config.Burst = 10
	}
	return nil
}
`
