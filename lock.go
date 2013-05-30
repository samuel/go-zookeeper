package zk

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var (
	ErrDeadlock  = errors.New("zk: trying to acquire a lock twice")
	ErrNotLocked = errors.New("zk: not locked")
)

type Lock struct {
	c    *Conn
	name string
	path string
	seq  int
	guid [16]byte
}

func NewLock(c *Conn, name string) *Lock {
	return &Lock{
		c:    c,
		name: name,
		path: "",
	}
}

func parseSeq(path string) (int, error) {
	parts := strings.Split(path, "-")
	return strconv.Atoi(parts[len(parts)-1])
}

func (l *Lock) Lock() error {
	if l.path != "" {
		return ErrDeadlock
	}

	_, err := io.ReadFull(rand.Reader, l.guid[:16])
	if err != nil {
		return err
	}

	basePath := fmt.Sprintf("/locks/%s", l.name)
	prefix := fmt.Sprintf("%s/%x-", basePath, l.guid)
	path, err := l.c.Create(prefix, []byte("lock"), FlagEphemeral|FlagSequence, WorldACL(PermAll))
	if err != nil {
		return err
	}
	// TODO: handle recoverable errors

	seq, err := parseSeq(path)
	if err != nil {
		return err
	}

	for {
		children, _, err := l.c.Children(basePath)
		if err != nil {
			return err
		}

		lowestSeq := seq
		prevSeq := 0
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

		exists, _, ch, err := l.c.ExistsW(prevSeqPath)
		if err != nil {
			return err
		}

		if exists {
			ev := <-ch
			if ev.Err != nil {
				return ev.Err
			}
		}
	}

	l.seq = seq
	l.path = path
	return nil
}

func (l *Lock) Unlock() error {
	if l.path == "" {
		return ErrNotLocked
	}
	if err := l.c.Delete(l.path, -1); err != nil {
		return err
	}
	l.path = ""
	l.seq = 0
	return nil
}
