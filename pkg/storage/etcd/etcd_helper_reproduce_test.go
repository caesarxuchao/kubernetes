package etcd

import (
	"context"
	"io/ioutil"
	"testing"
	"time"

	"github.com/coreos/etcd/client"
	"github.com/coreos/etcd/pkg/transport"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/testapi"
	"k8s.io/kubernetes/pkg/runtime"
	"k8s.io/kubernetes/pkg/storage"
	"k8s.io/kubernetes/pkg/storage/etcd/etcdtest"
)

func newHttpTransport(t *testing.T) client.CancelableTransport {
	tr, err := transport.NewTransport(transport.TLSInfo{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestCreatableNotUpdatable(t *testing.T) {
	tr := newHttpTransport(t)
	etcdclient, err := client.New(client.Config{Transport: tr, Endpoints: []string{"http://127.0.0.1:2379"}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := ioutil.ReadFile("large-pod.yml")
	if err != nil {
		t.Fatal(err)
	}
	pod := &api.Pod{}
	if err := runtime.DecodeInto(testapi.Default.Codec(), data, pod); err != nil {
		t.Fatal(err)
	}
	updatedPod := *pod
	updatedPod.ObjectMeta.Name = "newname"

	helper := newEtcdHelper(etcdclient, testapi.Default.Codec(), etcdtest.PathPrefix())
	returnedObj := &api.Pod{}
	if err = helper.Create(context.TODO(), "/some/key", pod, returnedObj, 5); err != nil {
		t.Fatal("Unexpected error %#v", err)
	}

	err = helper.GuaranteedUpdate(context.TODO(), "/some/key", &api.Pod{}, true, nil, storage.SimpleUpdate(func(in runtime.Object) (runtime.Object, error) {
		t.Logf("callback called")
		return &updatedPod, nil
	}))
	if err != nil {
		t.Errorf("Unexpected error %#v, error string: %s", err, err)
	}
}
