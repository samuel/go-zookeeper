package zk

import (
	"encoding/binary"
	"errors"
	"reflect"
	"runtime"
)

var (
	ErrUnhandledFieldType = errors.New("zk: unhandled field type")
	ErrPtrExpected        = errors.New("zk: encode/decode expect a non-nil pointer to struct")
	ErrShortBuffer        = errors.New("zk: buffer too small")
)

type ACL struct {
	Perms  int32
	Scheme string
	Id     string
}

type Stat struct {
	Czxid          int64
	Mzxid          int64
	Ctime          int64
	Mtime          int64
	Version        int32
	Cversion       int32
	Aversion       int32
	EphemeralOwner int64
	DataLength     int32
	NumChildren    int32
	Pzxid          int64
}

type requestHeader struct {
	Xid    int32
	Opcode int32
}

type responseHeader struct {
	Xid  int32
	Zxid int64
	Err  ErrCode
}

type multiHeader struct {
	Type int32
	Done bool
	Err  ErrCode
}

type auth struct {
	Type   int32
	Scheme string
	Auth   []byte
}

// Generic request structs

type pathRequest struct {
	Path string
}

type pathVersionRequest struct {
	Path    string
	Version int32
}

type pathWatchRequest struct {
	Path  string
	Watch bool
}

type pathResponse struct {
	Path string
}

type statResponse struct {
	Stat Stat
}

//

type checkVersionRequest pathVersionRequest
type closeRequest struct{}
type closeResponse struct{}

type connectRequest struct {
	ProtocolVersion int32
	LastZxidSeen    int64
	TimeOut         int32
	SessionId       int64
	Passwd          []byte
}

type connectResponse struct {
	ProtocolVersion int32
	TimeOut         int32
	SessionId       int64
	Passwd          []byte
}

type createRequest struct {
	Path  string
	Data  []byte
	Acl   []ACL
	Flags int32
}

type createResponse pathResponse
type deleteRequest pathVersionRequest
type deleteResponse struct{}

type errorResponse struct {
	Err int32
}

type existsRequest pathWatchRequest
type existsResponse statResponse
type getAclRequest pathRequest

type getAclResponse struct {
	Acl  []ACL
	Stat Stat
}

type getChildrenRequest pathRequest

type getChildrenResponse struct {
	Children []string
}

type getChildren2Request pathWatchRequest

type getChildren2Response struct {
	Children []string
	Stat     Stat
}

type getDataRequest pathWatchRequest

type getDataResponse struct {
	Data []byte
	Stat Stat
}

type getMaxChildrenRequest pathRequest

type getMaxChildrenResponse struct {
	Max int32
}

type getSaslRequest struct {
	Token []byte
}

type pingRequest struct{}
type pingResponse struct{}

type setAclRequest struct {
	Path    string
	Acl     []ACL
	Version int32
}

type setAclResponse statResponse

type setDataRequest struct {
	Path    string
	Data    []byte
	Version int32
}

type setDataResponse statResponse

type setMaxChildren struct {
	Path string
	Max  int32
}

type setSaslRequest struct {
	Token string
}

type setSaslResponse struct {
	Token string
}

type setWatchesRequest struct {
	RealtiveZxid int64
	DataWatches  []string
	ExistWatches []string
	ChildWatches []string
}

type setWatchesResponse struct{}

type syncRequest pathRequest
type syncResponse pathResponse

type watcherEvent struct {
	Type  EventType
	State State
	Path  string
}

func decodePacket(buf []byte, st interface{}) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(runtime.Error); ok && e.Error() == "runtime error: slice bounds out of range" {
				err = ErrShortBuffer
			} else {
				panic(r)
			}
		}
	}()

	v := reflect.ValueOf(st)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return 0, ErrPtrExpected
	}
	return decodePacketValue(buf, v.Elem())
}

func decodePacketValue(buf []byte, v reflect.Value) (int, error) {
	n := 0
	switch v.Kind() {
	default:
		return n, ErrUnhandledFieldType
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			n2, err := decodePacketValue(buf[n:], field)
			n += n2
			if err != nil {
				return n, err
			}
		}
	case reflect.Bool:
		v.SetBool(buf[n] != 0)
		n += 1
	case reflect.Int32:
		v.SetInt(int64(binary.BigEndian.Uint32(buf[n : n+4])))
		n += 4
	case reflect.Int64:
		v.SetInt(int64(binary.BigEndian.Uint64(buf[n : n+8])))
		n += 8
	case reflect.String:
		ln := int(binary.BigEndian.Uint32(buf[n : n+4]))
		v.SetString(string(buf[n+4 : n+4+ln]))
		n += 4 + ln
	case reflect.Slice:
		switch v.Type().Elem().Kind() {
		default:
			count := int(binary.BigEndian.Uint32(buf[n : n+4]))
			n += 4
			values := reflect.MakeSlice(v.Type(), count, count)
			v.Set(values)
			for i := 0; i < count; i++ {
				n2, err := decodePacketValue(buf[n:], values.Index(i))
				n += n2
				if err != nil {
					return n, err
				}
			}
		case reflect.Uint8:
			ln := int(binary.BigEndian.Uint32(buf[n : n+4]))
			if ln < 0 {
				n += 4
				v.SetBytes(nil)
			} else {
				bytes := make([]byte, ln)
				copy(bytes, buf[n+4:n+4+ln])
				v.SetBytes(bytes)
				n += 4 + ln
			}
		}
	}
	return n, nil
}

func encodePacket(buf []byte, st interface{}) (n int, err error) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(runtime.Error); ok && e.Error() == "runtime error: slice bounds out of range" {
				err = ErrShortBuffer
			} else {
				panic(r)
			}
		}
	}()

	v := reflect.ValueOf(st)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return 0, ErrPtrExpected
	}
	return encodePacketValue(buf, v.Elem())
}

func encodePacketValue(buf []byte, v reflect.Value) (int, error) {
	n := 0
	switch v.Kind() {
	default:
		return n, ErrUnhandledFieldType
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			n2, err := encodePacketValue(buf[n:], field)
			n += n2
			if err != nil {
				return n, err
			}
		}
	case reflect.Bool:
		if v.Bool() {
			buf[n] = 1
		} else {
			buf[n] = 0
		}
		n += 1
	case reflect.Int32:
		binary.BigEndian.PutUint32(buf[n:n+4], uint32(v.Int()))
		n += 4
	case reflect.Int64:
		binary.BigEndian.PutUint64(buf[n:n+8], uint64(v.Int()))
		n += 8
	case reflect.String:
		str := v.String()
		binary.BigEndian.PutUint32(buf[n:n+4], uint32(len(str)))
		copy(buf[n+4:n+4+len(str)], []byte(str))
		n += 4 + len(str)
	case reflect.Slice:
		switch v.Type().Elem().Kind() {
		default:
			count := v.Len()
			startN := n
			n += 4
			for i := 0; i < count; i++ {
				n2, err := encodePacketValue(buf[n:], v.Index(i))
				n += n2
				if err != nil {
					return n, err
				}
			}
			binary.BigEndian.PutUint32(buf[startN:startN+4], uint32(count))
		case reflect.Uint8:
			if v.IsNil() {
				binary.BigEndian.PutUint32(buf[n:n+4], uint32(0xffffffff))
				n += 4
			} else {
				bytes := v.Bytes()
				binary.BigEndian.PutUint32(buf[n:n+4], uint32(len(bytes)))
				copy(buf[n+4:n+4+len(bytes)], bytes)
				n += 4 + len(bytes)
			}
		}
	}
	return n, nil
}
