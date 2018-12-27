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


	// current process should win, test should hang here otherwise
	_ = <- c1

	kill := make(chan struct{}, 1)

	go func() {
		l2 := NewLeaderElector(zk, "/test", acls)
		c2, _ := l2.Start()
		defer l2.Stop()
		select {
			case <- c2:
				t.Error()
			case <- kill:
		}
	}()

	time.Sleep(time.Second)
	kill <- struct{}{}


	go func() {
		l3 := NewLeaderElector(zk, "/test", acls)
		c3, _ := l3.Start()

		defer l3.Stop()
		select {
		case <- c3:
			return
		case <- kill:
			t.Error()
		}
	}()

	l1.Stop()
	time.Sleep(time.Second)
	kill <- struct{}{}
}
