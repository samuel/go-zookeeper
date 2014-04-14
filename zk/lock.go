package zk

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrDeadlock  = errors.New("zk: trying to acquire a lock twice")
	ErrNotLocked = errors.New("zk: not locked")
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

func (l *Lock) SetTimeout(d time.Duration) {
	l.acquisitionTimeout = d
}

func (l *Lock) SetTTL(d time.Duration) {
	l.ttl = d
}

func (l *Lock) Lock() error {
	if l.lockPath != "" {
		return ErrDeadlock
	}

	// For backwards compatibility, if timeout is set but TTL isn't use the same value for both
	if l.ttl == 0 && l.acquisitionTimeout != 0 {
		l.ttl = l.acquisitionTimeout
	}

	prefix := fmt.Sprintf("%s/lock-", l.path)

	path := ""
	var err error
	for i := 0; i < 3; i++ {
		data := []byte{}
		if l.ttl > 0 {
			data, _ = time.Now().Add(l.ttl).GobEncode()
		}

		path, err = l.c.CreateProtectedEphemeralSequential(prefix, data, l.acl)
		if err == ErrNoNode {
			// Create parent node.
			parts := strings.Split(l.path, "/")
			pth := ""
			for _, p := range parts[1:] {
				pth += "/" + p
				_, err := l.c.Create(pth, []byte{}, 0, l.acl)
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

	var ttl time.Time
	for {
		children, _, err := l.c.Children(l.path)
		if err != nil {
			return err
		}

		lowestSeq := seq
		prevSeq := 0
		prevSeqPath := ""
		for _, p := range children {
			// check if this lock has timed out
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

		// Wait for a timeout period before giving up trying to achieve the lock
		var ev Event
		if l.acquisitionTimeout == 0 {
			ev = <-ch
		} else {
			select {
			case ev = <-ch:
			case <-time.After(l.acquisitionTimeout):
				// clean up after ourself
				if err := l.c.Delete(path, -1); err != nil {
					// Delete failed, so disconnect to trigger ephemeral nodes to clear
					if closeErr := l.c.conn.Close(); closeErr != nil {
						return fmt.Errorf("Failed to get lock after %v, then failed to delete ourself (%v), then failed to close connection (%v)", l.acquisitionTimeout, err, closeErr)
					} else {
						return fmt.Errorf("Failed to get lock after %v, then failed to delete ourself (%v)", l.acquisitionTimeout, err)
					}
				}
				return fmt.Errorf("Failed to get lock after %v", l.acquisitionTimeout)
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
