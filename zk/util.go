package zk

import (
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	DefaultRoot                 = "/"
	internalZookeeperNode       = "/zookeeper"
	internalZookeeperNodePrefix = "/zookeeper/"
)

// AuthACL produces an ACL list containing a single ACL which uses the
// provided permissions, with the scheme "auth", and ID "", which is used
// by ZooKeeper to represent any authenticated user.
func AuthACL(perms int32) []ACL {
	return []ACL{{perms, "auth", ""}}
}

// WorldACL produces an ACL list containing a single ACL which uses the
// provided permissions, with the scheme "world", and ID "anyone", which
// is used by ZooKeeper to represent any user at all.
func WorldACL(perms int32) []ACL {
	return []ACL{{perms, "world", "anyone"}}
}

func DigestACL(perms int32, user, password string) []ACL {
	userPass := []byte(fmt.Sprintf("%s:%s", user, password))
	h := sha1.New()
	if n, err := h.Write(userPass); err != nil || n != len(userPass) {
		panic("SHA1 failed")
	}
	digest := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return []ACL{{perms, "digest", fmt.Sprintf("%s:%s", user, digest)}}
}

// FormatServers takes a slice of addresses, and makes sure they are in a format
// that resembles <addr>:<port>. If the server has no port provided, the
// DefaultPort constant is added to the end.
func FormatServers(servers []string) []string {
	for i := range servers {
		if !strings.Contains(servers[i], ":") {
			servers[i] = servers[i] + ":" + strconv.Itoa(DefaultPort)
		}
	}
	return servers
}

// stringShuffle performs a Fisher-Yates shuffle on a slice of strings
func stringShuffle(s []string) {
	for i := len(s) - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
}

// validatePath will make sure a path is valid before sending the request
func validatePath(path string, isSequential bool) error {
	if path == "" {
		return ErrInvalidPath
	}

	if path[0] != '/' {
		return ErrInvalidPath
	}

	n := len(path)
	if n == 1 {
		// path is just the root
		return nil
	}

	if !isSequential && path[n-1] == '/' {
		return ErrInvalidPath
	}

	// Start at rune 1 since we already know that the first character is
	// a '/'.
	for i, w := 1, 0; i < n; i += w {
		r, width := utf8.DecodeRuneInString(path[i:])
		switch {
		case r == '\u0000':
			return ErrInvalidPath
		case r == '/':
			last, _ := utf8.DecodeLastRuneInString(path[:i])
			if last == '/' {
				return ErrInvalidPath
			}
		case r == '.':
			last, lastWidth := utf8.DecodeLastRuneInString(path[:i])

			// Check for double dot
			if last == '.' {
				last, _ = utf8.DecodeLastRuneInString(path[:i-lastWidth])
			}

			if last == '/' {
				if i+1 == n {
					return ErrInvalidPath
				}

				next, _ := utf8.DecodeRuneInString(path[i+w:])
				if next == '/' {
					return ErrInvalidPath
				}
			}
		case r >= '\u0000' && r <= '\u001f',
			r >= '\u007f' && r <= '\u009f',
			r >= '\uf000' && r <= '\uf8ff',
			r >= '\ufff0' && r < '\uffff':
			return ErrInvalidPath
		}
		w = width
	}
	return nil
}

// Additional utility functions inspired by the ZkUtil.java class that comes with the official Java client

// ListSubtree - BFS Traversal of the system under pathRoot, with the entries in the list, in the same order as that
// of the traversal.
//
// Important: This is not an atomic snapshot of the tree ever, but the state as it exists across multiple RPCs from
// zkClient to the ensemble.
func ListSubtree(zkConn *Conn, pathRoot string) ([]string, error) {
	queue := []string{pathRoot}
	tree := []string{pathRoot}
	var node string

	for {
		if len(queue) == 0 {
			// We're done
			return tree, nil
		}

		// Pop first element in the queue
		node, queue = queue[0], queue[1:]
		children, _, err := zkConn.Children(node)

		if err != nil {
			return nil, err
		}

		for _, child := range children {
			var childPath string

			if node == "/" {
				childPath = fmt.Sprintf("/%v", child)
			} else {
				childPath = fmt.Sprintf("%v/%v", node, child)
			}
			queue = append(queue, childPath)
			tree = append(tree, childPath)
		}

	}
}

// DeleteRecursively will recursively delete the node with the given path. All versions of all nodes under the given
// node are deleted.
//
// If there is an error with deleting one of the sub-nodes in the tree, this operation would abort and would be the
// responsibility of the caller to handle the same.
func DeleteRecursively(zkConn *Conn, pathRoot string) error {
	tree, err := ListSubtree(zkConn, pathRoot)
	if err != nil {
		return err
	}

	deletes := make([]interface{}, 0, len(tree))

	// We want to delete from the leaves
	for i := len(tree) - 1; i >= 0; i-- {
		if !IsInternalNode(tree[i]) && tree[i] != DefaultRoot {
			deletes = append(deletes, &DeleteRequest{Path: tree[i], Version: -1})
		}
	}

	// Atomically delete all nodes
	_, err = zkConn.Multi(deletes...)
	if err != nil {
		return err
	}

	return nil
}

func IsInternalNode(path string) bool {
	return path == internalZookeeperNode || strings.HasPrefix(path, internalZookeeperNodePrefix)
}
