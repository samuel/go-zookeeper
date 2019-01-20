package zk

import (
	"errors"
	"path"
	"strconv"
	"strings"
)

const (
	defaultLockName = "lock-"
)

var (
	// ErrDeadlock is returned by Lock when trying to lock twice without unlocking first
	ErrDeadlock = errors.New("zk: trying to acquire a lock twice")
	// ErrNotLocked is returned by Unlock when trying to release a lock that has not first be acquired.
	ErrNotLocked = errors.New("zk: not locked")

	defaultNewLockConstructorOptions = []LockConstructorOption{
		LockWithNameBuilder(defaultLockNameBuilder),
	}
)

// LockConstructorOption is used to provide optional constructor arguments to
// the NewLock constructor
type LockConstructorOption func(l *Lock)

// LockNameBuilder is a function which, when provided with a LockNameContext,
// will generate the name of the lock. While it is technically possible to
// create a lock name with slashes, it is discouraged.
//
// This option may be useful in cases when using the lock implementation along
// with other language implementations of Zookeeper locks. For instance, the
// python kazoo library creates locks with the name "__lock__" while this
// library uses "-lock-" by default.
type LockNameBuilder func(lockNameCtx LockNameBuilderContext) string

// Lock is a mutual exclusion lock.
type Lock struct {
	c        *Conn
	path     string
	acl      []ACL
	lockPath string
	seq      int
	name     string
}

// LockNameBuilderContext will be provided to any template specified in the
// LockWithNameBuilder constructor option in order to generate the name for the
// constructed lock
type LockNameBuilderContext struct {
	Path string
}

// LockWithNameBuilder creates a parameter which
func LockWithNameBuilder(b LockNameBuilder) LockConstructorOption {
	return func(l *Lock) {
		ctx := LockNameBuilderContext{
			Path: l.path,
		}
		l.name = b(ctx)
	}
}

// defaultLockNameBuilder wraps the default lock name as a builder for use as a
// default constructor option
func defaultLockNameBuilder(_ LockNameBuilderContext) string {
	return defaultLockName
}

// NewLock creates a new lock instance using the provided connection, path, and acl.
// The path must be a node that is only used by this lock. A lock instances starts
// unlocked until Lock() is called.
func NewLock(c *Conn, path string, acl []ACL, opts... LockConstructorOption,
	) *Lock {
	created := &Lock{
		c:    c,
		path: path,
		acl:  acl,
	}

	allOpts := append(defaultNewLockConstructorOptions, opts...)
	for _, opt := range allOpts {
		opt(created)
	}

	return created
}

func parseSeq(path string) (int, error) {
	parts := strings.Split(path, "-")
	return strconv.Atoi(parts[len(parts)-1])
}

// Lock attempts to acquire the lock. It will wait to return until the lock
// is acquired or an error occurs. If this instance already has the lock
// then ErrDeadlock is returned.
func (l *Lock) Lock() error {
	if l.lockPath != "" {
		return ErrDeadlock
	}

	prefix := path.Join(l.path, l.name)

	lockPath := ""
	var err error
	for i := 0; i < 3; i++ {
		lockPath, err = l.c.CreateProtectedEphemeralSequential(prefix, []byte{}, l.acl)
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

	seq, err := parseSeq(lockPath)
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
	l.lockPath = lockPath
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
	return nil
}
