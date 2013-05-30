package zk

import (
	"testing"
	"time"
)

func TestLock(t *testing.T) {
	zk, _, err := Connect([]string{"127.0.0.1:2182"}, time.Second*15)
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	l := NewLock(zk, "test")
	if err := l.Lock(); err != nil {
		t.Fatal(err)
	}
	if err := l.Unlock(); err != nil {
		t.Fatal(err)
	}

	val := make(chan int, 3)

	if err := l.Lock(); err != nil {
		t.Fatal(err)
	}

	l2 := NewLock(zk, "test")
	go func() {
		if err := l2.Lock(); err != nil {
			t.Fatal(err)
		}
		val <- 2
		if err := l2.Unlock(); err != nil {
			t.Fatal(err)
		}
		val <- 3
	}()
	time.Sleep(time.Millisecond * 100)

	val <- 1
	if err := l.Unlock(); err != nil {
		t.Fatal(err)
	}
	if x := <-val; x != 1 {
		t.Fatalf("Expected 1 instead of %d", x)
	}
	if x := <-val; x != 2 {
		t.Fatalf("Expected 2 instead of %d", x)
	}
	if x := <-val; x != 3 {
		t.Fatalf("Expected 3 instead of %d", x)
	}
}
