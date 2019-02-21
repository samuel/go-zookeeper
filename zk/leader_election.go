package zk

import (
	"fmt"
	"strings"
)

// LeaderElector defines methods for leader election
type LeaderElector interface {
	// Start leader election for current process, caller should listen to returned
	// chan for success event
	Start() (chan struct{}, error)
	// Stop election, resign leader if already chosen as leader
	Stop() error
}

// NewLeaderElector returns a LeaderElector implementation
func NewLeaderElector(conn *Conn, electionPath string, acl []ACL) LeaderElector {
	return &leaderElectorImpl{conn: conn, electionPath: electionPath, acl: acl}
}

type leaderElectorImpl struct {
	conn *Conn
	acl []ACL
	// path of election
	electionPath string
	// path and version of current process' znode
	nodePath string
	lock *Lock
}

func (l *leaderElectorImpl) Start() (election chan struct{}, err error) {
	l.lock = NewLock(l.conn, l.electionPath + "_lock", l.acl)
	err = l.lock.Lock()
	if err != nil {
		return
	}
	defer func() {
		err = l.lock.Unlock()
	}()

	election = make(chan struct{}, 1)
	prefix := fmt.Sprintf("%s/condidate-", l.electionPath)

	// create znode for current process
	nodePath := ""
	for i := 0; i< 3; i++ {
		nodePath, err = l.conn.CreateProtectedEphemeralSequential(prefix, []byte{}, l.acl)
		if err == ErrNoNode {
			// Create parent node.
			parts := strings.Split(l.electionPath, "/")
			pth := ""
			for _, p := range parts[1:] {
				var exists bool
				pth += "/" + p
				exists, _, err = l.conn.Exists(pth)
				if err != nil {
					return
				}
				if exists == true {
					continue
				}
				_, err = l.conn.Create(pth, []byte{}, 0, l.acl)
				if err != nil && err != ErrNodeExists {
					return
				}
			}
		} else if err == nil {
			break
		} else {
			return
		}
	}
	l.nodePath = nodePath
	go l.watchAndPipe(election)
	return
}

// setup watches, push to election chan when current process got elected
// this func is meant to be kept running in a separate goroutine
func (l *leaderElectorImpl) watchAndPipe(election chan struct{}) {
	// check up to 3 times for potential wins
	for i := 1; i < 3; i++ {
		seq, err := parseSeq(l.nodePath, "")
		if err != nil {
			return
		}

		children, _, err := l.conn.Children(l.electionPath)
		if err != nil {
			return
		}

		lowestSeq := seq
		prevSeq := -1
		prevSeqPath := ""
		for _, p := range children {
			s, err := parseSeq(p, "")
			if err != nil {
				return
			}
			if s < lowestSeq {
				lowestSeq = s
			}
			if s < seq && s > prevSeq {
				prevSeq = s
				prevSeqPath = p
			}
		}

		if seq == lowestSeq {
			// won election!
			election <- struct{}{}
			return
		}

		// Wait on the node next in line to be deleted
		_, _, ch, err := l.conn.GetW(l.electionPath + "/" + prevSeqPath)
		if err != nil && err != ErrNoNode {
			return
		}

		ev := <-ch
		if ev.Err != nil {
			err = ev.Err
			return
		}
		if ev.Type == EventNodeDeleted {
			// previous node deleted, could be a win, check again to make sure
			continue
		}
		break
	}
}

func (l *leaderElectorImpl) Stop() (err error) {
	err = l.lock.Lock()
	if err != nil {
		return
	}
	defer func() {
		err = l.lock.Unlock()
	}()

	if l.nodePath != "" {
		var stat *Stat
		_, stat, err = l.conn.Get(l.nodePath)
		if err != nil {
			return
		}

		err = l.conn.Delete(l.nodePath, stat.Version)
	}
	return
}

