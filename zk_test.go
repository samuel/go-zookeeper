package zk

import (
	"testing"
	"time"
)

func TestCreate(t *testing.T) {
	zk, chEvent, err := Connect([]string{"127.0.0.1:2182"}, time.Second*15)
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	if err := zk.Delete("/gozk-test", -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}
	if path, err := zk.Create("/gozk-test", []byte{1, 2, 3, 4}, 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("Create returned error: %+v", err)
	} else if path != "/gozk-test" {
		t.Fatalf("Create returned different path %s != '/gozk-test'", path)
	}
	_ = chEvent
	time.Sleep(time.Second)
}
