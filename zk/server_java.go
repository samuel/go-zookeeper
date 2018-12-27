package zk

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type ErrMissingServerConfigField string

func (e ErrMissingServerConfigField) Error() string {
	return fmt.Sprintf("zk: missing server config field '%s'", string(e))
}

const (
	DefaultServerTickTime                 = 2000
	DefaultServerInitLimit                = 10
	DefaultServerSyncLimit                = 5
	DefaultServerAutoPurgeSnapRetainCount = 3
	DefaultPeerPort                       = 2888
	DefaultLeaderElectionPort             = 3888
)

type server struct {
	stdout, stderr io.Writer
	cmdString      string
	cmdArgs        []string

	cmd *exec.Cmd
	// this cancel will kill the command being run in this case the server itself.
	cancelFunc context.CancelFunc
}

func NewIntegrationTestServer(t *testing.T, configPath string, stdout, stderr io.Writer) (*server, error) {
	// allow external systems to configure this zk server bin path.
	zkPath := os.Getenv("ZOOKEEPER_BIN_PATH")
	if zkPath == "" {
		// default to a static reletive path that can be setup with a build system
		zkPath = "../zookeeper/bin"
	}
	if _, err := os.Stat(zkPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("zk: could not find testing zookeeper bin path at %q: %v ", zkPath, err)
		}
	}

	return &server{
		cmdString: filepath.Join(zkPath, "zkServer.sh"),
		cmdArgs:   []string{"start-foreground", configPath},
		stdout:    stdout, stderr: stderr,
	}, nil
}

func (srv *server) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	srv.cancelFunc = cancel

	srv.cmd = exec.CommandContext(ctx, srv.cmdString, srv.cmdArgs...)
	srv.cmd.Stdout = srv.stdout
	srv.cmd.Stderr = srv.stderr
	return srv.cmd.Start()
}

func (srv *server) Stop() error {
	srv.cancelFunc()
	return srv.cmd.Wait()
}

type ServerConfigServer struct {
	ID                 int
	Host               string
	PeerPort           int
	LeaderElectionPort int
}

type ServerConfig struct {
	TickTime                 int    // Number of milliseconds of each tick
	InitLimit                int    // Number of ticks that the initial synchronization phase can take
	SyncLimit                int    // Number of ticks that can pass between sending a request and getting an acknowledgement
	DataDir                  string // Direcrory where the snapshot is stored
	ClientPort               int    // Port at which clients will connect
	AutoPurgeSnapRetainCount int    // Number of snapshots to retain in dataDir
	AutoPurgePurgeInterval   int    // Purge task internal in hours (0 to disable auto purge)
	Servers                  []ServerConfigServer
}

func (sc ServerConfig) Marshall(w io.Writer) error {
	// the admin server is not wanted in test cases as it slows the startup process and is
	// of little unit test value.
	fmt.Fprintln(w, "admin.enableServer=false")
	if sc.DataDir == "" {
		return ErrMissingServerConfigField("dataDir")
	}
	fmt.Fprintf(w, "dataDir=%s\n", sc.DataDir)
	if sc.TickTime <= 0 {
		sc.TickTime = DefaultServerTickTime
	}
	fmt.Fprintf(w, "tickTime=%d\n", sc.TickTime)
	if sc.InitLimit <= 0 {
		sc.InitLimit = DefaultServerInitLimit
	}
	fmt.Fprintf(w, "initLimit=%d\n", sc.InitLimit)
	if sc.SyncLimit <= 0 {
		sc.SyncLimit = DefaultServerSyncLimit
	}
	fmt.Fprintf(w, "syncLimit=%d\n", sc.SyncLimit)
	if sc.ClientPort <= 0 {
		sc.ClientPort = DefaultPort
	}
	fmt.Fprintf(w, "clientPort=%d\n", sc.ClientPort)
	if sc.AutoPurgePurgeInterval > 0 {
		if sc.AutoPurgeSnapRetainCount <= 0 {
			sc.AutoPurgeSnapRetainCount = DefaultServerAutoPurgeSnapRetainCount
		}
		fmt.Fprintf(w, "autopurge.snapRetainCount=%d\n", sc.AutoPurgeSnapRetainCount)
		fmt.Fprintf(w, "autopurge.purgeInterval=%d\n", sc.AutoPurgePurgeInterval)
	}
	if len(sc.Servers) > 0 {
		for _, srv := range sc.Servers {
			if srv.PeerPort <= 0 {
				srv.PeerPort = DefaultPeerPort
			}
			if srv.LeaderElectionPort <= 0 {
				srv.LeaderElectionPort = DefaultLeaderElectionPort
			}
			fmt.Fprintf(w, "server.%d=%s:%d:%d\n", srv.ID, srv.Host, srv.PeerPort, srv.LeaderElectionPort)
		}
	}
	return nil
}
