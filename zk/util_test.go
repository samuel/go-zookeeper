package zk

import (
	"reflect"
	"testing"
)

func TestFormatServers(t *testing.T) {
	t.Parallel()
	servers := []string{"127.0.0.1:2181", "127.0.0.42", "127.0.42.1:8811"}
	r := []string{"127.0.0.1:2181", "127.0.0.42:2181", "127.0.42.1:8811"}
	for i, s := range FormatServers(servers) {
		if s != r[i] {
			t.Errorf("%v should equal %v", s, r[i])
		}
	}
}

func TestValidatePath(t *testing.T) {
	tt := []struct {
		path  string
		seq   bool
		valid bool
	}{
		{"/this is / a valid/path", false, true},
		{"/", false, true},
		{"", false, false},
		{"not/valid", false, false},
		{"/ends/with/slash/", false, false},
		{"/sequential/", true, true},
		{"/test\u0000", false, false},
		{"/double//slash", false, false},
		{"/single/./period", false, false},
		{"/double/../period", false, false},
		{"/double/..ok/period", false, true},
		{"/double/alsook../period", false, true},
		{"/double/period/at/end/..", false, false},
		{"/name/with.period", false, true},
		{"/test\u0001", false, false},
		{"/test\u001f", false, false},
		{"/test\u0020", false, true}, // first allowable
		{"/test\u007e", false, true}, // last valid ascii
		{"/test\u007f", false, false},
		{"/test\u009f", false, false},
		{"/test\uf8ff", false, false},
		{"/test\uffef", false, true},
		{"/test\ufff0", false, false},
	}

	for _, tc := range tt {
		err := validatePath(tc.path, tc.seq)
		if (err != nil) == tc.valid {
			t.Errorf("failed to validate path %q", tc.path)
		}
	}
}

func TestListSubtree(t *testing.T) {
	ts, err := StartTestCluster(1, nil, logWriter{t: t, p: "[ZKERR] "})
	assertNoErrors(t, err)
	defer ts.Stop()

	conn, err := ts.Connect(0)
	assertNoErrors(t, err)
	defer conn.Close()

	createEmptyNodeOrFail(t, conn, "/abc")
	createEmptyNodeOrFail(t, conn, "/abc/123")
	createEmptyNodeOrFail(t, conn, "/abc/123/xyz")
	createEmptyNodeOrFail(t, conn, "/def")
	createEmptyNodeOrFail(t, conn, "/foo")
	createEmptyNodeOrFail(t, conn, "/foo/bar")
	createEmptyNodeOrFail(t, conn, "/foo/baz")

	subtree, err := ListSubtree(conn, DefaultRoot)
	assertNoErrors(t, err)

	expected := []string{"/", "/abc", "/zookeeper", "/def", "/foo", "/abc/123", "/zookeeper/quota", "/foo/bar", "/foo/baz", "/abc/123/xyz"}

	if !reflect.DeepEqual(expected, subtree) {
		t.Fatalf("Expected %v, got %v", expected, subtree)
	}

	subtree, err = ListSubtree(conn, "/foo")
	assertNoErrors(t, err)

	expected = []string{"/foo", "/foo/bar", "/foo/baz"}

	if !reflect.DeepEqual(expected, subtree) {
		t.Fatalf("Expected %v, got %v", expected, subtree)
	}
}

func TestDeleteRecursively(t *testing.T) {
	ts, err := StartTestCluster(1, nil, logWriter{t: t, p: "[ZKERR] "})
	assertNoErrors(t, err)
	defer ts.Stop()

	conn, err := ts.Connect(0)
	assertNoErrors(t, err)
	defer conn.Close()

	createEmptyNodeOrFail(t, conn, "/abc")
	createEmptyNodeOrFail(t, conn, "/abc/123")
	createEmptyNodeOrFail(t, conn, "/abc/123/1")
	createEmptyNodeOrFail(t, conn, "/abc/123/1/a")
	createEmptyNodeOrFail(t, conn, "/abc/123/2")
	createEmptyNodeOrFail(t, conn, "/abc/123/2/b")
	createEmptyNodeOrFail(t, conn, "/abc/123/3")
	createEmptyNodeOrFail(t, conn, "/abc/123/3/d")
	createEmptyNodeOrFail(t, conn, "/abc/123/3/d/e")
	createEmptyNodeOrFail(t, conn, "/abc/123/3/d/e/f")

	subtree, err := ListSubtree(conn, DefaultRoot)
	assertNoErrors(t, err)

	expected := []string{"/", "/abc", "/zookeeper", "/abc/123", "/zookeeper/quota", "/abc/123/1", "/abc/123/2",
		"/abc/123/3", "/abc/123/1/a", "/abc/123/2/b", "/abc/123/3/d", "/abc/123/3/d/e", "/abc/123/3/d/e/f"}

	if !reflect.DeepEqual(expected, subtree) {
		t.Fatalf("Expected %v, got %v", expected, subtree)
	}

	err = DeleteRecursively(conn, "/abc/123/3")
	assertNoErrors(t, err)

	subtree, err = ListSubtree(conn, DefaultRoot)
	assertNoErrors(t, err)

	expected = []string{"/", "/abc", "/zookeeper", "/abc/123", "/zookeeper/quota", "/abc/123/1", "/abc/123/2",
		"/abc/123/1/a", "/abc/123/2/b"}

	if !reflect.DeepEqual(expected, subtree) {
		t.Fatalf("Expected %v, got %v", expected, subtree)
	}

	err = DeleteRecursively(conn, DefaultRoot)
	assertNoErrors(t, err)

	subtree, err = ListSubtree(conn, DefaultRoot)
	assertNoErrors(t, err)

	expected = []string{"/", "/zookeeper", "/zookeeper/quota"}

	if !reflect.DeepEqual(expected, subtree) {
		t.Fatalf("Expected %v, got %v", expected, subtree)
	}
}

func TestIsInternalNode(t *testing.T) {
	if IsInternalNode("/abc") {
		t.Error("/abc is not an internal node")
	}

	if IsInternalNode("/zookeeperstuff") {
		t.Error("/zookeeperstuff is not an internal node")
	}

	if !IsInternalNode("/zookeeper") {
		t.Error("/zookeeper is an internal node")
	}

	if !IsInternalNode("/zookeeper/something") {
		t.Error("/zookeeper/something is an internal node")
	}
}

func assertNoErrors(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Caught unexpected error: %v", err)
	}
}

func createEmptyNodeOrFail(t *testing.T, conn *Conn, path string) {
	_, err := conn.Create(path, []byte{0}, 0, WorldACL(PermAll))
	assertNoErrors(t, err)
}
