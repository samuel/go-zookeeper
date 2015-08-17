package zk

import (
	"testing"
	"time"
)

func setupChrootTest(t *testing.T, ts *TestCluster, chroot string) (conn, chrootedConn *Conn) {
	zk, _, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}

	zkChroot, _, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	zkChroot.SetChroot(chroot)

	// create chroot
	if _, err := zk.Create(chroot, []byte{}, 0, WorldACL(PermAll)); err != nil && err != ErrNodeExists {
		t.Fatalf("unable to create root %s, err=%q", chroot, err)
	}
	return zk, zkChroot
}

func teardownChrootTest(t *testing.T, conn, chrootedConn *Conn, chroot string) {
	err := conn.Delete(chroot, -1)
	if err != nil {
		t.Logf("failed deleting chroot in cleanup, err=%q", err)
	}
	conn.Close()
	chrootedConn.Close()
}

func TestChrootCreate(t *testing.T) {
	chroot := "/testchroot"
	ts, err := StartTestCluster(3, nil, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, zkChroot := setupChrootTest(t, ts, chroot)
	defer teardownChrootTest(t, zk, zkChroot, chroot)

	// create node in chroot
	created, err := zkChroot.Create("/node", []byte{}, 0, WorldACL(PermAll))
	if err != nil {
		t.Fatalf("create err=%q", err)
	}
	defer zkChroot.Delete("/node", -1)
	if created != "/node" {
		t.Errorf("create return err have=%q, want=%q", created, "/node")
	}

	// check if node is visible to non-
	exists, _, err := zk.Exists("/testchroot/node")
	if err != nil {
		t.Fatalf("exists err=%q", err)
	}
	if !exists {
		t.Errorf("could not find /testchroot/node")
	}
}

func TestChrootWatch(t *testing.T) {
	chroot := "/testchroot"
	ts, err := StartTestCluster(3, nil, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()

	zk, zkChroot := setupChrootTest(t, ts, chroot)
	defer teardownChrootTest(t, zk, zkChroot, chroot)

	// verify that watches work
	_, _, ch, err := zkChroot.ExistsW("/watched")
	if err != nil {
		t.Fatalf("unable to set chrooted watch err=%q", err)
	}

	_, err = zk.Create("/testchroot/watched", []byte{}, 0, WorldACL(PermAll))
	if err != nil {
		t.Fatalf("unable to create chrooted watch node err=%q", err)
	}
	defer zk.Delete("/testchroot/watched", -1)

	select {
	case ev := <-ch:
		if ev.Path != "/watched" {
			t.Errorf("unexpected path from chroot watcher - have=%q, want=%q", ev.Path, "/watched")
		}
	case <-time.After(time.Second):
		t.Errorf("chrooted watcher did not receive notification about /testchroot/watched")
	}

}

func TestChrootChildrenW(t *testing.T) {
	chroot := "/testchroot"
	ts, err := StartTestCluster(3, nil, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()
	zk, zkChroot := setupChrootTest(t, ts, chroot)
	defer teardownChrootTest(t, zk, zkChroot, chroot)

	// verify that children watches work
	_, _, ch, err := zkChroot.ChildrenW("")
	if err != nil {
		t.Fatalf("unable to set chrooted children watch err=%q", err)
	}
	_, err = zk.Create("/testchroot/child", []byte{}, 0, WorldACL(PermAll))
	if err != nil {
		t.Fatalf("unable to create chrooted child node err=%q", err)
	}
	defer zk.Delete("/testchroot/child", -1)

	select {
	case ev := <-ch:
		if ev.Path != "" {
			t.Errorf("unexpected path from chroot watcher - have=%q, want=%q", ev.Path, "/watched")
		}
	case <-time.After(time.Second):
		t.Errorf("chrooted children watcher did not receive notification about /testchroot/child")
	}
}

func TestEmptyPath(t *testing.T) {
	chroot := "/testchroot"
	ts, err := StartTestCluster(3, nil, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()

	zk, zkChroot := setupChrootTest(t, ts, chroot)
	defer teardownChrootTest(t, zk, zkChroot, chroot)

	// attempting to get the empty path should be an error on a normal
	// client
	_, err = zk.Set("", []byte{}, -1)
	if err != ErrInvalidPath {
		t.Errorf("unexpected err when trying to Get \"\", have=%q want=ErrNoNode", err)
	}
	// when chrooted, it should be allowed
	_, err = zkChroot.Set("", []byte{}, -1)
	if err != nil {
		t.Errorf("unexpected err when trying to Get \"\", have=%q want=<nil>", err)
	}
}
