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

package main

import (
	"crypto/tls"
	"crypto/x509"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/golang/glog"
)

// Get a clientset with in-cluster config.
func getClient() *kubernetes.Clientset {
	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatal(err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatal(err)
	}
	return clientset
}

func configTLS(config Config, clientset *kubernetes.Clientset) *tls.Config {
	sCert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		glog.Fatal(err)
	}
	apiserverCA := x509.NewCertPool()
	apiserverCA.AppendCertsFromPEM(cert)
	return &tls.Config{
		Certificates: []tls.Certificate{sCert},
		// TODO: uses mutual tls after we agree on what cert the apiserver should use.
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  apiserverCA,
	}
}

// CHAO: manually copied from $K1/certs/caCert.pem
var cert = []byte(`-----BEGIN CERTIFICATE-----
MIIDNjCCAh6gAwIBAgIJALcjsj2XH8TDMA0GCSqGSIb3DQEBCwUAMC8xLTArBgNV
BAMMJGdlbmVyaWNfd2ViaG9va19hZG1pc3Npb25fZXhhbXBsZV9jYTAgFw0xODAz
MDgyMzA1MTNaGA8yMjkxMTIyMjIzMDUxM1owLzEtMCsGA1UEAwwkZ2VuZXJpY193
ZWJob29rX2FkbWlzc2lvbl9leGFtcGxlX2NhMIIBIjANBgkqhkiG9w0BAQEFAAOC
AQ8AMIIBCgKCAQEApzK5TMl3hl1j60ZXhDS8H9QMKLL+IITI0L1QROEp/HeBUmdw
ay+uQc6De+2M+YKtGNW+9uKlQrEnrFk5GSYRC9C1I/YdZoNctSAW4ifuXF4vr34N
XEwjZWP2w41josLLoXSUhono6ow6DHwurKg1PaUbS/gK8xbJn7FwuYK3olQRzluS
p6ImMn1cxtLrS4Yj3O2DE+0/VuL/13JFYj9DSgkbGua0O8/m5wWt0XbgTv49UUY4
AUa9OiNrjh05xgbyJYpA06qVA8jJSy9ZS27oyZw3ZdPWlbTJjDfQybtTvAlWFkAS
Ah4s1guNdpK/75/pzfVcm/arWguG38xoOpfY0QIDAQABo1MwUTAdBgNVHQ4EFgQU
wGmTa/ylhJYdaaVK7IwnE8P7SeIwHwYDVR0jBBgwFoAUwGmTa/ylhJYdaaVK7Iwn
E8P7SeIwDwYDVR0TAQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEACw2x86g+
GiZ0Gvb+GINdzNg2tgTttNofqxW2V8sbknpidF8RYrQJo+NawLiilYkAVKKNgsQb
4RZDzJWpD0j7bRLWIjUM0WwJm4/ozr2h1gqwa666bKmOj1HtPqNbPSbvwZFTXgFz
C+KuQ3nx3swsiN2ghtPqk3DtAzZx4ZKgkb74U0yKb67BMaw18jNQIwBdDXQTi5Yo
wzWQVagSlXYVUAIK0vKAwyQHauIZ4FuUNlY9Q/8Oldqx2Qe4W4foxLGKAUDi29Yj
cWb+NXNnylJOzkGiWm/0wQp/gTyXJE7MBPETq5OOkWZ3BIUlYYfiEKBJNc/cF5Po
+6U2nPSD8z6aFA==
-----END CERTIFICATE-----`)
