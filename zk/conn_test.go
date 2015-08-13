package zk

import "testing"

func TestChroot(t *testing.T) {
	ts, err := StartTestCluster(3, nil, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()

	zk, _, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	zkChroot, _, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zkChroot.Close()
	zkChroot.SetChroot("/testchroot")

	// create chroot
	if _, err := zk.Create("/testchroot", []byte{}, 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("unable to create /testchroot, err=%q", err)
	}
	defer func() {
		_, stat, _ := zk.Get("/testchroot")
		err := zk.Delete("/testchroot", stat.Version)
		if err != nil {
			t.Errorf("failed deleting /testchroot, err=%q", err)
		}
	}()

	// create node in chroot
	_, err = zkChroot.Create("/node", []byte{}, 0, WorldACL(PermAll))
	if err != nil {
		t.Fatalf("create err=%q", err)
	}
	defer func() {
		_, stat, _ := zkChroot.Get("/node")
		err := zkChroot.Delete("/node", stat.Version)
		if err != nil {
			t.Errorf("failed deleting /testchroot/node, err=%q", err)
		}
	}()

	// check if node is visible
	exists, _, err := zk.Exists("/testchroot/node")
	if err != nil {
		t.Fatalf("exists err=%q", err)
	}
	if !exists {
		t.Errorf("could not find /testchroot/node")
	}

}
