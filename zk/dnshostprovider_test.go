package zk

import (
	"fmt"
	"testing"
	"time"
)

// localhostLookupHost is a test replacement for net.LookupHost that
// always returns 127.0.0.1
func localhostLookupHost(host string) ([]string, error) {
	return []string{"127.0.0.1"}, nil
}

// TestDNSHostProviderCreate is just like TestCreate, but with an
// overridden HostProvider that ignores the provided hostname.
func TestDNSHostProviderCreate(t *testing.T) {
	ts, err := StartTestCluster(1, nil, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()

	port := ts.Servers[0].Port
	server := fmt.Sprintf("foo.example.com:%d", port)
	hostProvider := &DNSHostProvider{lookupHost: localhostLookupHost}
	zk, _, err := Connect([]string{server}, time.Second*15, WithHostProvider(hostProvider))
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

// Test the `retryStart` functionality of DNSHostProvider.
func TestDNSHostProviderRetryStart(t *testing.T) {
	hp := &DNSHostProvider{lookupHost: func(host string) ([]string, error) {
		return []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}, nil
	}}

	if err := hp.Init([]string{"foo.example.com:12345"}); err != nil {
		t.Fatal(err)
	}

	testdata := []struct {
		retryStartWant bool
		callConnected  bool
	}{
		// Repeated failures.
		{false, false},
		{false, false},
		{false, false},
		{true, false},
		{false, false},
		{false, false},
		{true, true},

		// One success offsets things.
		{false, false},
		{false, true},
		{false, true},

		// Repeated successes.
		{false, true},
		{false, true},
		{false, true},
		{false, true},
		{false, true},

		// And some more failures.
		{false, false},
		{false, false},
		{true, false}, // Looped back to last known good server: all alternates failed.
		{false, false},
	}

	for i, td := range testdata {
		_, retryStartGot := hp.Next()
		if retryStartGot != td.retryStartWant {
			t.Errorf("%d: retryStart=%v; want %v", i, retryStartGot, td.retryStartWant)
		}
		if td.callConnected {
			hp.Connected()
		}
	}
}
