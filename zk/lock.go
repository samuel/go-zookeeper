package zk

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrDeadlock    = errors.New("zk: trying to acquire a lock twice")
	ErrNotLocked   = errors.New("zk: not locked")
	ErrLockTimeout = errors.New("zk: timeout trying to acquire lock")
)

type Lock struct {
	c                  *Conn
	path               string
	acl                []ACL
	lockPath           string
	seq                int
	acquisitionTimeout time.Duration
	ttl                time.Duration
}

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

// SetTimeout sets the time we will wait to acquire the lock, bail out if time is exceeded
func (l *Lock) SetTimeout(d time.Duration) {
	l.acquisitionTimeout = d
}

// SetTTL sets the max time that the lock should live for
func (l *Lock) SetTTL(d time.Duration) {
	l.ttl = d
}

// Lock attempts to acquire a lock.
// Uses the recipe http://zookeeper.apache.org/doc/trunk/recipes.html#sc_recipes_Locks.
// Timeout achieved by first creating the lock node and then waiting X secs. If lock not yet acquired it will delete and then return.
// Does not account for initial request taking too long
func (l *Lock) Lock() error {
	if l.lockPath != "" {
		return ErrDeadlock
	}

	prefix := fmt.Sprintf("%s/lock-", l.path)

	path := ""
	var err error
	data := []byte{}
	if l.ttl > 0 {
		data, _ = time.Now().Add(l.ttl).GobEncode()
	}
	maxEnd := time.Now().Add(l.acquisitionTimeout)
	path, err = l.c.CreateProtectedEphemeralSequential(prefix, data, l.acl)
	if err == ErrNoNode {
		// Create parent nodes and try again
		parts := strings.Split(l.path, "/")
		pth := ""
		for _, p := range parts[1:] {
			pth += "/" + p
			_, err := l.c.Create(pth, []byte{}, 0, l.acl)
			if err != nil && err != ErrNodeExists {
				// Timeout error could be here too
				return err
			}
		}
		path, err = l.c.CreateProtectedEphemeralSequential(prefix, data, l.acl)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	seq, err := parseSeq(path)
	if err != nil {
		return err
	}

	// wait for lock to be acquired
	for {
		lowestSeq, prevSeqPath, err := l.findLowestSequenceNode(seq)
		if seq == lowestSeq {
			// Acquired the lock
			break
		}

		// Wait on the node next in line for the lock by setting watcher
		_, _, ch, err := l.c.GetW(l.path + "/" + prevSeqPath)
		if err != nil && err != ErrNoNode {
			return err
		} else if err != nil && err == ErrNoNode {
			// try again
			continue
		}

		// Wait for a timeout period before giving up trying to achieve the lock
		var ev Event
		if l.acquisitionTimeout == 0 {
			ev = <-ch // watch event fired
		} else {
			select {
			case ev = <-ch: // watch event fired
			case <-time.After(maxEnd.Sub(time.Now())): // handles -ve
				// clean up after ourself
				return l.cleanUpTimeoutLock(path)
			}
		}

		if ev.Err != nil {
			return ev.Err
		}
	}
	l.seq = seq
	l.lockPath = path
	return nil
}

func (l *Lock) findLowestSequenceNode(seq int) (lowestSeq int, prevSeqPath string, err error) {
	children, _, err := l.c.Children(l.path)
	if err != nil {
		return -1, "", err
	}

	var ttl time.Time
	lowestSeq = seq
	prevSeq := 0
	prevSeqPath = ""
	for _, p := range children {
		// check if this lock has timed out TODO keep this?
		data, _, _ := l.c.Get(l.path + "/" + p)
		if len(data) > 0 {
			ttl.GobDecode(data)
			if ttl.Before(time.Now()) {
				l.c.Delete(l.path+"/"+p, -1)
				continue
			}
		}

		s, err := parseSeq(p)
		if err != nil {
			return -1, "", err
		}
		if s < lowestSeq {
			lowestSeq = s
		}
		if s < seq && s > prevSeq {
			prevSeq = s
			prevSeqPath = p
		}
	}
	return lowestSeq, prevSeqPath, err
}

func (l *Lock) cleanUpTimeoutLock(path string) error {
	if err := l.c.Delete(path, -1); err != nil {
		// Delete failed, so disconnect to trigger ephemeral nodes to clear
		if closeErr := l.c.conn.Close(); closeErr != nil {
			return fmt.Errorf("Failed to get lock after %v, then failed to delete ourself (%v), then failed to close connection (%v)", l.acquisitionTimeout, err, closeErr)
		} else {
			return fmt.Errorf("Failed to get lock after %v, then failed to delete ourself (%v)", l.acquisitionTimeout, err)
		}
	}
	return ErrLockTimeout
}

func (l *Lock) Unlock() error {
	if l.lockPath == "" {
		return ErrNotLocked
	}
	if err := l.c.Delete(l.lockPath, -1); err != nil {
		return err
	}
	l.lockPath = ""
	l.seq = 0
	return nil
}
