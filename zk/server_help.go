package zk

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type TestServer struct {
	Port int
	Path string
	Srv  *server
}

type TestCluster struct {
	Path    string
	Servers []TestServer
}

func StartTestCluster(t *testing.T, size int, stdout, stderr io.Writer) (*TestCluster, error) {
	if testing.Short() {
		t.Skip("ZK clsuter tests skipped in short case.")
	}
	var (
		err error
	)
	// this will be set by systems like travis_ci to be able to test multiple versions
	// testingServerVersion := os.Getenv("zk_version")
	// if testingServerVersion != "" {
	// 	// will look like a semver ie. 3.4.1 or 3.5.4-beta
	// 	parts := strings.Split(testingServerVersion, ".")
	// 	if len(parts) >= 2 {
	// 		// minor version we switch on
	// 		version, err = strconv.Atoi(parts[1])
	// 		if err != nil {
	// 			t.Fatalf("failed to detect zk minor version from environment: %v", err)
	// 		}
	// 	}
	// }

	tmpPath, err := ioutil.TempDir("", "gozk")
	if err != nil {
		t.Fatalf("failed to create tmp fir for test server setup: %v", err)
	}

	success := false
	startPort := int(rand.Int31n(6000) + 10000)
	cluster := &TestCluster{Path: tmpPath}
	defer func() {
		if !success {
			cluster.Stop()
		}
	}()
	for serverN := 0; serverN < size; serverN++ {
		srvPath := filepath.Join(tmpPath, fmt.Sprintf("srv%d", serverN))
		if err := os.Mkdir(srvPath, 0700); err != nil {
			t.Fatalf("failed to make server path: %v", err)
		}

		port := startPort + serverN*3
		cfg := ServerConfig{
			ClientPort: port,
			DataDir:    srvPath,
		}
		for i := 0; i < size; i++ {
			cfg.Servers = append(cfg.Servers, ServerConfigServer{
				ID:                 i + 1,
				Host:               "127.0.0.1",
				PeerPort:           startPort + i*3 + 1,
				LeaderElectionPort: startPort + i*3 + 2,
			})
		}

		cfgPath := filepath.Join(srvPath, "zoo.cfg")
		fi, err := os.Create(cfgPath)
		if err != nil {
			return nil, err
		}
		err = cfg.Marshall(fi)
		fi.Close()
		if err != nil {
			return nil, err
		}

		fi, err = os.Create(filepath.Join(srvPath, "myid"))
		if err != nil {
			return nil, err
		}
		_, err = fmt.Fprintf(fi, "%d\n", serverN+1)
		fi.Close()
		if err != nil {
			return nil, err
		}

		var srv *server
		// if version == 0 || version < 5 {
		// 	// we default assume we want a 3.4 cluster
		// 	srv, err = New3dot4TestServer(t, cfgPath, stdout, stderr)
		// 	if err != nil {
		// 		return nil, err
		// 	}
		// } else {
		srv, err = New3dot5TestServer(t, cfgPath, stdout, stderr)
		if err != nil {
			return nil, err
		}
		// }

		if err := srv.Start(); err != nil {
			return nil, err
		}

		cluster.Servers = append(cluster.Servers, TestServer{
			Path: srvPath,
			Port: cfg.ClientPort,
			Srv:  srv,
		})
	}

	if err := cluster.waitForStart(20, time.Second); err != nil {
		return nil, err
	}

	success = true
	return cluster, nil
}

func (tc *TestCluster) Connect(idx int) (*Conn, error) {
	zk, _, err := Connect([]string{fmt.Sprintf("127.0.0.1:%d", tc.Servers[idx].Port)}, time.Second*15)
	return zk, err
}

func (tc *TestCluster) ConnectAll() (*Conn, <-chan Event, error) {
	return tc.ConnectAllTimeout(time.Second * 15)
}

func (tc *TestCluster) ConnectAllTimeout(sessionTimeout time.Duration) (*Conn, <-chan Event, error) {
	return tc.ConnectWithOptions(sessionTimeout)
}

func (tc *TestCluster) ConnectWithOptions(sessionTimeout time.Duration, options ...connOption) (*Conn, <-chan Event, error) {
	hosts := make([]string, len(tc.Servers))
	for i, srv := range tc.Servers {
		hosts[i] = fmt.Sprintf("127.0.0.1:%d", srv.Port)
	}
	zk, ch, err := Connect(hosts, sessionTimeout, options...)
	return zk, ch, err
}

func (tc *TestCluster) Stop() error {
	for _, srv := range tc.Servers {
		srv.Srv.Stop()
	}
	defer os.RemoveAll(tc.Path)
	return tc.waitForStop(5, time.Second)
}

// waitForStart blocks until the cluster is up
func (tc *TestCluster) waitForStart(maxRetry int, interval time.Duration) error {
	// verify that the servers are up with SRVR
	serverAddrs := make([]string, len(tc.Servers))
	for i, s := range tc.Servers {
		serverAddrs[i] = fmt.Sprintf("127.0.0.1:%d", s.Port)
	}

	for i := 0; i < maxRetry; i++ {
		_, ok := FLWSrvr(serverAddrs, time.Second)
		if ok {
			return nil
		}
		time.Sleep(interval)
	}
	return fmt.Errorf("unable to verify health of servers")
}

// waitForStop blocks until the cluster is down
func (tc *TestCluster) waitForStop(maxRetry int, interval time.Duration) error {
	// verify that the servers are up with RUOK
	serverAddrs := make([]string, len(tc.Servers))
	for i, s := range tc.Servers {
		serverAddrs[i] = fmt.Sprintf("127.0.0.1:%d", s.Port)
	}

	var success bool
	for i := 0; i < maxRetry && !success; i++ {
		success = true
		for _, ok := range FLWRuok(serverAddrs, time.Second) {
			if ok {
				success = false
			}
		}
		if !success {
			time.Sleep(interval)
		}
	}
	if !success {
		return fmt.Errorf("unable to verify servers are down")
	}
	return nil
}

func (tc *TestCluster) StartServer(server string) {
	for _, s := range tc.Servers {
		if strings.HasSuffix(server, fmt.Sprintf(":%d", s.Port)) {
			s.Srv.Start()
			return
		}
	}
	panic(fmt.Sprintf("Unknown server: %s", server))
}

func (tc *TestCluster) StopServer(server string) {
	for _, s := range tc.Servers {
		if strings.HasSuffix(server, fmt.Sprintf(":%d", s.Port)) {
			s.Srv.Stop()
			return
		}
	}
	panic(fmt.Sprintf("Unknown server: %s", server))
}

func (tc *TestCluster) StartAllServers() error {
	for _, s := range tc.Servers {
		if err := s.Srv.Start(); err != nil {
			return fmt.Errorf(
				"Failed to start server listening on port `%d` : %+v", s.Port, err)
		}
	}

	return nil
}

func (tc *TestCluster) StopAllServers() error {
	for _, s := range tc.Servers {
		if err := s.Srv.Stop(); err != nil {
			return fmt.Errorf(
				"Failed to stop server listening on port `%d` : %+v", s.Port, err)
		}
	}

	return nil
}
