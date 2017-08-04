package zk

import "testing"

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
		valid bool
	}{
		{"/this is / a valid/path", true},
		{"/", true},
		{"", false},
		{"not/valid", false},
		{"/ends/with/slash/", false},
		{"/test\u0000", false},
		{"/double//slash", false},
		{"/single/./period", false},
		{"/double/../period", false},
		{"/double/..ok/period", true},
		{"/double/alsook../period", true},
		{"/double/period/at/end/..", false},
		{"/name/with.period", true},
		{"/test\u0001", false},
		{"/test\u001f", false},
		{"/test\u0020", true}, // first allowable
		{"/test\u007e", true}, // last valid ascii
		{"/test\u007f", false},
		{"/test\u009f", false},
		{"/test\uf8ff", false},
		{"/test\uffef", true},
		{"/test\ufff0", false},
	}

	for _, tc := range tt {
		err := validatePath(tc.path)
		if (err != nil) == tc.valid {
			t.Errorf("failed to validate path %q", tc.path)
		}
	}
}
