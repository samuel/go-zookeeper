package zk

import (
	"testing"
)

func TestBasicCluster(t *testing.T) {
	ts, err := StartTestCluster(3)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk1, err := ts.Connect(0)
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk1.Close()
	zk2, err := ts.Connect(1)
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk2.Close()

	if _, err := zk1.Create("/gozk-test", []byte("foo-cluster"), 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("Create failed on node 1: %+v", err)
	}
	if by, _, err := zk2.Get("/gozk-test"); err != nil {
		t.Fatalf("Get failed on node 2: %+v", err)
	} else if string(by) != "foo-cluster" {
		t.Fatal("Wrong data for node 2")
	}
}

func TestClientClusterFailover(t *testing.T) {
	ts, err := StartTestCluster(3)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	if _, err := zk.Create("/gozk-test", []byte("foo-cluster"), 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("Create failed on node 1: %+v", err)
	}

	ts.Servers[0].Srv.Stop()

	if by, _, err := zk.Get("/gozk-test"); err != nil {
		t.Fatalf("Get failed on node 2: %+v", err)
	} else if string(by) != "foo-cluster" {
		t.Fatal("Wrong data for node 2")
	}
}
