package zk

import (
	"crypto/tls"
	"io/ioutil"
	"os"
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

func TestStateChangesTLS(t *testing.T) {
	if os.Getenv("ZK_VERSION") != "3.5.6" {
		t.Skip("No TLS support")
	}

	config, err := newTLSConfig("/tmp/certs/client.cer.pem", "/tmp/certs/client.key.pem")
	if err != nil {
		panic(err)
	}

	ts, err := StartTestCluster(t, 1, logWriter{t: t, p: "[ZK] "}, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()

	callbackChan := make(chan Event)
	f := func(event Event) {
		callbackChan <- event
	}

	zk, eventChan, err := ts.ConnectWithOptionsTLS(15*time.Second, config, WithEventCallback(f))
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}

	verifyEventOrder := func(c <-chan Event, expectedStates []State, source string) {
		for _, state := range expectedStates {
			for {
				event, ok := <-c
				if !ok {
					t.Fatalf("unexpected channel close for %s", source)
				}

				if event.Type != EventSession {
					continue
				}

				if event.State != state {
					t.Fatalf("mismatched state order from %s, expected %v, received %v", source, state, event.State)
				}
				break
			}
		}
	}

	states := []State{StateConnecting, StateConnected, StateHasSession}
	verifyEventOrder(callbackChan, states, "callback")
	verifyEventOrder(eventChan, states, "event channel")

	zk.Close()
	verifyEventOrder(callbackChan, []State{StateDisconnected}, "callback")
	verifyEventOrder(eventChan, []State{StateDisconnected}, "event channel")
}

func newTLSConfig(clientCertFile, clientKeyFile string) (*tls.Config, error) {
	tlsConfig := tls.Config{
		InsecureSkipVerify: true,
	}

	// Load client cert
	cert, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
	if err != nil {
		return nil, err
	}
	tlsConfig.Certificates = []tls.Certificate{cert}

	tlsConfig.BuildNameToCertificate()
	return &tlsConfig, nil
}
