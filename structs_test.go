package zk

import (
	"reflect"
	"testing"
)

func TestEncodeDecodePacket(t *testing.T) {
	encodeDecodeTest(t, &requestHeader{-2, 5})
	encodeDecodeTest(t, &connectResponse{1, 2, 3, nil})
	encodeDecodeTest(t, &connectResponse{1, 2, 3, []byte{4, 5, 6}})
	encodeDecodeTest(t, &getAclResponse{[]ACL{{12, "s", "anyone"}}, Stat{}})
	encodeDecodeTest(t, &getChildrenResponse{[]string{"foo", "bar"}})
	encodeDecodeTest(t, &pathWatchRequest{"path", true})
	encodeDecodeTest(t, &pathWatchRequest{"path", false})
}

func encodeDecodeTest(t *testing.T, r interface{}) {
	buf := make([]byte, 1024)
	n, err := encodePacket(buf, r)
	if err != nil {
		t.Errorf("encodePacket returned non-nil error %+v\n", err)
		return
	}
	r2 := reflect.New(reflect.ValueOf(r).Elem().Type()).Interface()
	n2, err := decodePacket(buf, r2)
	if err != nil {
		t.Errorf("decodePacket returned non-nil error %+v\n", err)
		return
	}
	if n != n2 {
		t.Errorf("sizes don't match: %d != %d", n, n2)
		return
	}
	if !reflect.DeepEqual(r, r2) {
		t.Errorf("results don't match: %+v != %+v", r, r2)
		return
	}
}
