package zk

import (
	"io/ioutil"
	"testing"
	"time"
)

func TestRecurringReAuthHang(t *testing.T) {
	t.Skip("Race condition in test")

	sessionTimeout := 2 * time.Second

	finish := make(chan struct{})
	defer close(finish)
	go func() {
		select {
		case <-finish:
			return
		case <-time.After(5 * sessionTimeout):
			panic("expected not hang")
		}
	}()

	zkC, err := StartTestCluster(t, 2, ioutil.Discard, ioutil.Discard)
	if err != nil {
		panic(err)
	}
	defer zkC.Stop()

	conn, evtC, err := zkC.ConnectAll()
	if err != nil {
		panic(err)
	}
	for conn.State() != StateHasSession {
		time.Sleep(50 * time.Millisecond)
	}

	go func() {
		for range evtC {
		}
	}()

	// Add auth.
	conn.AddAuth("digest", []byte("test:test"))

	currentServer := conn.Server()
	conn.debugCloseRecvLoop = true
	conn.debugReauthDone = make(chan struct{})
	zkC.StopServer(currentServer)
	// wait connect to new zookeeper.
	for conn.Server() == currentServer && conn.State() != StateHasSession {
		time.Sleep(100 * time.Millisecond)
	}

	<-conn.debugReauthDone
}

func TestClean(t *testing.T) {
	ts, err := StartTestCluster(t, 1, nil, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()

	zk, _, err := ts.ConnectAll()
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	acls := WorldACL(PermAll)
	_, err = zk.Create("/cleanup", []byte{}, 0, acls)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	path, err := zk.CreateProtectedEphemeralSequential("/cleanup/lock-", []byte{}, acls)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	zk.cleanupChan <- path

	exists, _, evCh, err := zk.ExistsW(path)
	if exists {
		select {
		case ev := <-evCh:
			if ev.Err != nil {
				t.Fatalf("ExistW event returned with error %v", err)
			}
			if ev.Type != EventNodeDeleted {
				t.Fatal("Wrong event received")
			}
		case <-time.After(1 * time.Second):
			t.Fatal("Node is not cleared")
		}
	}
}