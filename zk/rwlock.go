// reference: https://zookeeper.apache.org/doc/current/recipes.html#Shared+Locks
// usage: one RWLock can only be used with 1 Lock() + Unlock() call OR 1 RLock() + RUnlock() call
// so for each reads, create a new ZKRWLock instance, same for writes

package zk

import (
	"fmt"
	"strings"

	"github.com/samuel/go-zookeeper/zk"
)

const (
	readSeqPrefix  = "read"
	writeSeqPrefix = "write"
)

// ZKRWLock defines distributed lock implemented with zk
type ZKRWLock interface {
	Lock() error
	Unlock() error
	RLock() error
	RUnlock() error
}

// NewZKRWLock returns a new ZKRWLock
func NewZKRWLock(c *zk.Conn, path string, acl []zk.ACL) ZKRWLock {
	return &zkRWLockImpl{
		conn: c,
		path: path,
		acl:  acl,
	}
}

type zkRWLockImpl struct {
	conn          *zk.Conn
	path          string
	acl           []zk.ACL
	writeLockPath string
	readLockPath  string
}

func (l *zkRWLockImpl) Lock() error {
	if l.writeLockPath != "" {
		return zk.ErrDeadlock
	}

	prefix := fmt.Sprintf("%s/%s-", l.path, writeSeqPrefix)

	path := ""
	var err error
	for i := 0; i < 3; i++ {
		path, err = l.conn.CreateProtectedEphemeralSequential(prefix, []byte{}, l.acl)
		if err == zk.ErrNoNode {
			// Create parent node.
			parts := strings.Split(l.path, "/")
			pth := ""
			for _, p := range parts[1:] {
				var exists bool
				pth += "/" + p
				exists, _, err = l.conn.Exists(pth)
				if err != nil {
					return err
				}
				if exists == true {
					continue
				}
				_, err = l.conn.Create(pth, []byte{}, 0, l.acl)
				if err != nil && err != zk.ErrNodeExists {
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

	seq, err := parseSeq(path, writeSeqPrefix)
	if err != nil {
		return err
	}

	for {
		children, _, err := l.conn.Children(l.path)
		if err != nil {
			return err
		}

		lowestSeq := seq
		prevSeq := -1
		prevSeqPath := ""
		for _, p := range children {
			s, err := parseSeq(p, "")
			if err != nil {
				return err
			}
			if s >= 0 && s < lowestSeq {
				lowestSeq = s
			}
			if s >= 0 && s < seq && s > prevSeq {
				prevSeq = s
				prevSeqPath = p
			}
		}
		if seq == lowestSeq {
			// Acquired the lock
			break
		}

		// Wait on the node next in line for the lock
		_, _, ch, err := l.conn.GetW(l.path + "/" + prevSeqPath)
		if err != nil && err != zk.ErrNoNode {
			return err
		} else if err != nil && err == zk.ErrNoNode {
			// try again
			continue
		}

		ev := <-ch
		if ev.Err != nil {
			return ev.Err
		}
	}

	l.writeLockPath = path
	return nil
}

func (l *zkRWLockImpl) Unlock() error {
	if l.writeLockPath == "" {
		return zk.ErrNotLocked
	}
	if err := l.conn.Delete(l.writeLockPath, -1); err != nil {
		return err
	}
	l.writeLockPath = ""
	return nil
}

func (l *zkRWLockImpl) RLock() error {
	if l.readLockPath != "" {
		return zk.ErrDeadlock
	}

	prefix := fmt.Sprintf("%s/%s-", l.path, readSeqPrefix)

	path := ""
	var err error
	for i := 0; i < 3; i++ {
		path, err = l.conn.CreateProtectedEphemeralSequential(prefix, []byte{}, l.acl)
		if err == zk.ErrNoNode {
			// Create parent node.
			parts := strings.Split(l.path, "/")
			pth := ""
			for _, p := range parts[1:] {
				var exists bool
				pth += "/" + p
				exists, _, err = l.conn.Exists(pth)
				if err != nil {
					return err
				}
				if exists == true {
					continue
				}
				_, err = l.conn.Create(pth, []byte{}, 0, l.acl)
				if err != nil && err != zk.ErrNodeExists {
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

	seq, err := parseSeq(path, readSeqPrefix)
	if err != nil {
		return err
	}

	for {
		children, _, err := l.conn.Children(l.path)
		if err != nil {
			return err
		}

		lowestSeq := seq
		prevSeq := -1
		prevSeqPath := ""
		for _, p := range children {
			s, err := parseSeq(p, writeSeqPrefix)
			if err != nil {
				return err
			}
			if s >= 0 && s < lowestSeq {
				lowestSeq = s
			}
			if s >= 0 && s < seq && s > prevSeq {
				prevSeq = s
				prevSeqPath = p
			}
		}

		if seq == lowestSeq {
			// Acquired the lock
			break
		}

		// Wait on the node next in line for the lock
		_, _, ch, err := l.conn.GetW(l.path + "/" + prevSeqPath)
		if err != nil && err != zk.ErrNoNode {
			return err
		} else if err != nil && err == zk.ErrNoNode {
			// try again
			continue
		}

		ev := <-ch
		if ev.Err != nil {
			return ev.Err
		}
	}

	l.readLockPath = path
	return nil
}
func (l *zkRWLockImpl) RUnlock() error {
	if l.readLockPath == "" {
		return zk.ErrNotLocked
	}
	if err := l.conn.Delete(l.readLockPath, -1); err != nil {
		return err
	}
	l.readLockPath = ""
	return nil
}

