package zk

import (
	"testing"
	"time"
)

func TestCreate(t *testing.T) {
	ts, err := StartTestCluster(1)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	path := "/gozk-test"

	if err := zk.Delete(path, -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}
	if p, err := zk.Create(path, []byte{1, 2, 3, 4}, 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("Create returned error: %+v", err)
	} else if p != path {
		t.Fatalf("Create returned different path '%s' != '%s'", p, path)
	}
	if data, stat, err := zk.Get(path); err != nil {
		t.Fatalf("Get returned error: %+v", err)
	} else if stat == nil {
		t.Fatal("Get returned nil stat")
	} else if len(data) < 4 {
		t.Fatal("Get returned wrong size data")
	}
}

func TestMulti(t *testing.T) {
	ts, err := StartTestCluster(1)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	path := "/gozk-test"

	if err := zk.Delete(path, -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}
	ops := MultiOps{
		Create: []CreateRequest{
			{Path: path, Data: []byte{1, 2, 3, 4}, Acl: WorldACL(PermAll)},
		},
		SetData: []SetDataRequest{
			{Path: path, Data: []byte{1, 2, 3, 4}, Version: -1},
		},
		// Delete: []DeleteRequest{
		// 	{Path: path, Version: -1},
		// },
	}
	if err := zk.Multi(ops); err != nil {
		t.Fatalf("Multi returned error: %+v", err)
	}
	if data, stat, err := zk.Get(path); err != nil {
		t.Fatalf("Get returned error: %+v", err)
	} else if stat == nil {
		t.Fatal("Get returned nil stat")
	} else if len(data) < 4 {
		t.Fatal("Get returned wrong size data")
	}
}

func TestGetSetACL(t *testing.T) {
	ts, err := StartTestCluster(1)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	if err := zk.AddAuth("digest", []byte("blah")); err != nil {
		t.Fatalf("AddAuth returned error %+v", err)
	}

	path := "/gozk-test"

	if err := zk.Delete(path, -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}
	if path, err := zk.Create(path, []byte{1, 2, 3, 4}, 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("Create returned error: %+v", err)
	} else if path != "/gozk-test" {
		t.Fatalf("Create returned different path '%s' != '/gozk-test'", path)
	}

	expected := WorldACL(PermAll)

	if acl, stat, err := zk.GetACL(path); err != nil {
		t.Fatalf("GetACL returned error %+v", err)
	} else if stat == nil {
		t.Fatalf("GetACL returned nil Stat")
	} else if len(acl) != 1 || expected[0] != acl[0] {
		t.Fatalf("GetACL mismatch expected %+v instead of %+v", expected, acl)
	}

	expected = []ACL{{PermAll, "ip", "127.0.0.1"}}

	if stat, err := zk.SetACL(path, expected, -1); err != nil {
		t.Fatalf("SetACL returned error %+v", err)
	} else if stat == nil {
		t.Fatalf("SetACL returned nil Stat")
	}

	if acl, stat, err := zk.GetACL(path); err != nil {
		t.Fatalf("GetACL returned error %+v", err)
	} else if stat == nil {
		t.Fatalf("GetACL returned nil Stat")
	} else if len(acl) != 1 || expected[0] != acl[0] {
		t.Fatalf("GetACL mismatch expected %+v instead of %+v", expected, acl)
	}
}

func TestAuth(t *testing.T) {
	ts, err := StartTestCluster(1)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	path := "/gozk-digest-test"
	if err := zk.Delete(path, -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}

	acl := DigestACL(PermAll, "user", "password")

	if p, err := zk.Create(path, []byte{1, 2, 3, 4}, 0, acl); err != nil {
		t.Fatalf("Create returned error: %+v", err)
	} else if p != path {
		t.Fatalf("Create returned different path '%s' != '%s'", p, path)
	}

	if a, stat, err := zk.GetACL(path); err != nil {
		t.Fatalf("GetACL returned error %+v", err)
	} else if stat == nil {
		t.Fatalf("GetACL returned nil Stat")
	} else if len(a) != 1 || acl[0] != a[0] {
		t.Fatalf("GetACL mismatch expected %+v instead of %+v", acl, a)
	}

	if _, _, err := zk.Get(path); err != ErrNoAuth {
		t.Fatalf("Get returned error %+v instead of ErrNoAuth", err)
	}

	if err := zk.AddAuth("digest", []byte("user:password")); err != nil {
		t.Fatalf("AddAuth returned error %+v", err)
	}

	if data, stat, err := zk.Get(path); err != nil {
		t.Fatalf("Get returned error %+v", err)
	} else if stat == nil {
		t.Fatalf("Get returned nil Stat")
	} else if len(data) != 4 {
		t.Fatalf("Get returned wrong data length")
	}
}

func TestChildWatch(t *testing.T) {
	ts, err := StartTestCluster(1)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	if err := zk.Delete("/gozk-test", -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}

	children, stat, childCh, err := zk.ChildrenW("/")
	if err != nil {
		t.Fatalf("Children returned error: %+v", err)
	} else if stat == nil {
		t.Fatal("Children returned nil stat")
	} else if len(children) < 1 {
		t.Fatal("Children should return at least 1 child")
	}

	if path, err := zk.Create("/gozk-test", []byte{1, 2, 3, 4}, 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("Create returned error: %+v", err)
	} else if path != "/gozk-test" {
		t.Fatalf("Create returned different path '%s' != '/gozk-test'", path)
	}

	select {
	case ev := <-childCh:
		if ev.Err != nil {
			t.Fatalf("Child watcher error %+v", ev.Err)
		}
		if ev.Path != "/" {
			t.Fatalf("Child watcher wrong path %s instead of %s", ev.Path, "/")
		}
	case _ = <-time.After(time.Second * 2):
		t.Fatal("Child watcher timed out")
	}
}

func TestSetWatchers(t *testing.T) {
	ts, err := StartTestCluster(1)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	zk.reconnectDelay = time.Second

	zk2, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk2.Close()

	if err := zk.Delete("/gozk-test", -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}

	testPath, err := zk.Create("/gozk-test-2", []byte{}, 0, WorldACL(PermAll))
	if err != nil {
		t.Fatalf("Create returned: %+v", err)
	}

	_, _, testEvCh, err := zk.GetW(testPath)
	if err != nil {
		t.Fatalf("GetW returned: %+v", err)
	}

	children, stat, childCh, err := zk.ChildrenW("/")
	if err != nil {
		t.Fatalf("Children returned error: %+v", err)
	} else if stat == nil {
		t.Fatal("Children returned nil stat")
	} else if len(children) < 1 {
		t.Fatal("Children should return at least 1 child")
	}

	zk.conn.Close()
	if err := zk2.Delete(testPath, -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}
	time.Sleep(time.Millisecond * 100)

	if path, err := zk2.Create("/gozk-test", []byte{1, 2, 3, 4}, 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("Create returned error: %+v", err)
	} else if path != "/gozk-test" {
		t.Fatalf("Create returned different path '%s' != '/gozk-test'", path)
	}

	select {
	case ev := <-testEvCh:
		if ev.Err != nil {
			t.Fatalf("GetW watcher error %+v", ev.Err)
		}
		if ev.Path != testPath {
			t.Fatalf("GetW watcher wrong path %s instead of %s", ev.Path, testPath)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("GetW watcher timed out")
	}

	select {
	case ev := <-childCh:
		if ev.Err != nil {
			t.Fatalf("Child watcher error %+v", ev.Err)
		}
		if ev.Path != "/" {
			t.Fatalf("Child watcher wrong path %s instead of %s", ev.Path, "/")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Child watcher timed out")
	}
}

func TestExpiringWatch(t *testing.T) {
	ts, err := StartTestCluster(1)
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	if err := zk.Delete("/gozk-test", -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}

	children, stat, childCh, err := zk.ChildrenW("/")
	if err != nil {
		t.Fatalf("Children returned error: %+v", err)
	} else if stat == nil {
		t.Fatal("Children returned nil stat")
	} else if len(children) < 1 {
		t.Fatal("Children should return at least 1 child")
	}

	zk.sessionId = 99999
	zk.conn.Close()

	select {
	case ev := <-childCh:
		if ev.Err != ErrSessionExpired {
			t.Fatalf("Child watcher error %+v instead of expected ErrSessionExpired", ev.Err)
		}
		if ev.Path != "/" {
			t.Fatalf("Child watcher wrong path %s instead of %s", ev.Path, "/")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Child watcher timed out")
	}
}
