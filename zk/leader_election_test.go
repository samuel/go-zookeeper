package zk

import (
	"testing"
	"time"
)

func TestLeaderElector(t *testing.T) {
	ts, err := StartTestCluster(1, nil, logWriter{t: t, p: "[ZKERR] "})
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

	l1 := NewLeaderElector(zk, "/test", acls)
	c1, err := l1.Start()
	if err != nil {
		t.Error(err)
	}


	// case1: l1 should win as only candidate, test should hang here otherwise
	_ = <- c1

	kill1 := make(chan struct{}, 1)

	go func() {
		// case 2: l2 should never win since l1 holds election
		l2 := NewLeaderElector(zk, "/test", acls)
		c2, _ := l2.Start()
		defer l2.Stop()
		select {
			case <- c2:
				t.Error()
			case <- kill1:
				return
		}
	}()

	time.Sleep(time.Second)
	// l2 exited election here
	kill1 <- struct{}{}


	kill2 := make(chan struct{}, 1)
	go func() {
		// case 3: l3 will win after l1 quit
		l3 := NewLeaderElector(zk, "/test", acls)
		c3, _ := l3.Start()

		defer l3.Stop()
		select {
		case <- c3:
			return
		case <- kill2:
			t.Error()
		}
	}()

	l1.Stop()
	time.Sleep(time.Second)
	kill2 <- struct{}{}

	kill3 := make(chan struct{}, 1)
	// case 4: l1 holds election, l3 waits on l2, and should not win in case l2
	// disconnect (l1 still holds election)
	c1, _ = l1.Start()
	_ = <- c1
	l2 := NewLeaderElector(zk, "/test", acls)
	_, _ = l2.Start()
	go func() {
		l3 := NewLeaderElector(zk, "/test", acls)
		c3, _ := l3.Start()

		defer l3.Stop()
		select {
		case <- c3:
			t.Error()
		case <- kill3:
			return
		}
	}()
	time.Sleep(time.Second)
	l2.Stop()
	time.Sleep(time.Second)
	kill3 <- struct{}{}
}
