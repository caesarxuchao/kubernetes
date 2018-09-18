package eventratelimit

import (
	"fmt"
	"strings"
	"testing"
)

// pass
var internalConfig = `apiVersion: eventratelimit.admission.k8s.io/__internal
kind: Configuration
limits:
- type: Namespace
  qps: 50
  burst: 100
  cacheSize: 2000
- type: User
  qps: 10
  burst: 50`

// pass
var v1alpha1Config = `apiVersion: eventratelimit.admission.k8s.io/v1alpha1
kind: Configuration
limits:
- type: Namespace
  qps: 50
  burst: 100
  cacheSize: 2000
- type: User
  qps: 10
  burst: 50`

// This one fails with:
// no kind "Configuration" is registered for version "eventratelimit.admission.k8s.io/v2"
var v2Config = `apiVersion: eventratelimit.admission.k8s.io/v2
kind: Configuration
limits:
- type: Namespace
  qps: 50
  burst: 100
  cacheSize: 2000
- type: User
  qps: 10
  burst: 50`

func TestLoadConfiguration(t *testing.T) {
	for _, config := range []string{internalConfig, v1alpha1Config, v2Config} {
		r := strings.NewReader(config)
		c, err := LoadConfiguration(r)
		if err != nil {
			t.Error(err)
		}
		fmt.Printf("c=%#v\n", c)
	}

}
