package zk

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	// ErrDeadlock is returned by Lock when trying to lock twice without unlocking first
	ErrDeadlock = errors.New("zk: trying to acquire a lock twice")
	// ErrNotLocked is returned by Unlock when trying to release a lock that has not first be acquired.
	ErrNotLocked = errors.New("zk: not locked")

)

// Lock is a mutual exclusion lock.
type Lock struct {
	c        *Conn
	path     string
	acl      []ACL
	lockPath string
	seq      int
}

var mapEphermeralSequenceLockPath = make(map[string]string)

// NewLock creates a new lock instance using the provided connection, path, and acl.
// The path must be a node that is only used by this lock. A lock instances starts
// unlocked until Lock() is called.
func NewLock(c *Conn, path string, acl []ACL) *Lock {
	return &Lock{
		c:    c,
		path: path,
		acl:  acl,
	}
}

func parseSeq(path string) (int, error) {
	parts := strings.Split(path, "-")
	return strconv.Atoi(parts[len(parts)-1])
}

// Lock attempts to acquire the lock. It will wait to return until the lock
// is acquired or an error occurs. If this instance already has the lock
// then ErrDeadlock is returned.
func (l *Lock) Lock() error {
	if(mapEphermeralSequenceLockPath[l.path]!="") {
		if(lockExists(l.c,l.path,mapEphermeralSequenceLockPath[l.path])) {
			return nil
		}
	}

	if l.lockPath != "" {
		return ErrDeadlock
	}

	prefix := fmt.Sprintf("%s/lock-", l.path)

	path := ""
	var err error
	for i := 0; i < 3; i++ {
		path, err = l.c.CreateProtectedEphemeralSequential(prefix, []byte{}, l.acl)
		mapEphermeralSequenceLockPath[l.path] = path
		if err == ErrNoNode {
			// Create parent node.
			parts := strings.Split(l.path, "/")
			pth := ""
			for _, p := range parts[1:] {
				var exists bool
				pth += "/" + p
				exists, _, err = l.c.Exists(pth)
				if err != nil {
					return err
				}
				if exists == true {
					continue
				}
				_, err = l.c.Create(pth, []byte{}, 0, l.acl)
				if err != nil && err != ErrNodeExists {
					return err
				}
			}
		} else if err == nil {
			break
		} else {
			return err
		}
	}
	if err != nil {
		return err
	}

	seq, err := parseSeq(path)
	if err != nil {
		return err
	}

	for {
		children, _, err := l.c.Children(l.path)
		if err != nil {
			return err
		}

		lowestSeq := seq
		prevSeq := -1
		prevSeqPath := ""
		for _, p := range children {
			s, err := parseSeq(p)
			if err != nil {
				return err
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
			// Acquired the lock
			break
		}

		// Wait on the node next in line for the lock
		_, _, ch, err := l.c.GetW(l.path + "/" + prevSeqPath)
		if err != nil && err != ErrNoNode {
			return err
		} else if err != nil && err == ErrNoNode {
			// try again
			continue
		}

		ev := <-ch
		if ev.Err != nil {
			return ev.Err
		}
	}

	l.seq = seq
	l.lockPath = path
	return nil
}

// Unlock releases an acquired lock. If the lock is not currently acquired by
// this Lock instance than ErrNotLocked is returned.
func (l *Lock) Unlock() error {
	if l.lockPath == "" {
		return ErrNotLocked
	}
	if err := l.c.Delete(l.lockPath, -1); err != nil {
		return err
	}
	l.lockPath = ""
	l.seq = 0
	delete(mapEphermeralSequenceLockPath,l.path)
	return nil
}

//Checks whether lock got created and response was lost because of network partition failure.
//It queries zookeeper and scans existing sequential ephemeral znodes under the parent path
//It finds out that previously requested sequence number corresponds to child having lowest sequence number
func lockExists(c *Conn, rootPath string, znodePath string) bool {
	seq, err := parseSeq(znodePath)
	if err != nil {
		return false
	}

	//scans the existing znodes if there are any
	for {
		children, _, err := c.Children(rootPath)
		if err != nil {
			return false
		}

		lowestSeq := seq
		prevSeq := -1
		for _, p := range children {
			s, err := parseSeq(p)
			if err != nil {
				return false
			}
			if s < lowestSeq {
				lowestSeq = s
			}
			if s < seq && s > prevSeq {
				prevSeq = s
			}
		}

		if seq == lowestSeq {
			// Acquired the lock
			return true
		} else {
			return false
		}
	}
}