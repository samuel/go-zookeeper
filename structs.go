package zk

import (
	"encoding/binary"
	"errors"
	"reflect"
)

type id struct {
	Scheme string
	Id     int32
}

type acl struct {
	Perms int32
	Id    id
}

type stat struct {
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
	Err  int32
}

type multiHeader struct {
	Type int32
	Done bool
	Err  int32
}

type auth struct {
	Type   int32
	Scheme string
	Auth   []byte
}

// Generic request structs

type emptyRequest struct {
	requestHeader
}

type pathRequest struct {
	requestHeader
	Path string
}

type pathVersionRequest struct {
	requestHeader
	Path    string
	Version int32
}

type pathWatchRequest struct {
	requestHeader
	Path  string
	Watch bool
}

type emptyResponse struct {
	responseHeader
}

type pathResponse struct {
	responseHeader
	Path string
}

type statResponse struct {
	responseHeader
	Stat stat
}

//

type checkVersionRequest pathVersionRequest
type closeRequest emptyRequest
type closeResponse emptyResponse

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
	requestHeader
	Path  string
	Data  []byte
	Acl   []acl
	Flags int32
}

type createResponse pathResponse
type deleteRequest pathVersionRequest
type deleteResponse emptyResponse

type errorResponse struct {
	responseHeader
	Err int32
}

type existsRequest pathWatchRequest
type existsResponse statResponse
type getAclRequest pathRequest

type getAclResponse struct {
	responseHeader
	Acl  []acl
	Stat stat
}

type getChildrenRequest pathRequest

type getChildrenResponse struct {
	responseHeader
	Children []string
}

type getChildren2Request pathWatchRequest

type getChildren2Response struct {
	responseHeader
	Children []string
	Stat     stat
}

type getDataRequest pathWatchRequest

type getDataResponse struct {
	responseHeader
	Data []byte
	Stat stat
}

type getMaxChildrenRequest pathRequest

type getMaxChildrenResponse struct {
	responseHeader
	Max int32
}

type getSaslRequest struct {
	requestHeader
	Token []byte
}

type pingRequest emptyRequest
type pingResponse emptyResponse

type setAclRequest struct {
	requestHeader
	Path    string
	Acl     []acl
	Version int32
}

type setAclResponse statResponse

type setDataRequest struct {
	requestHeader
	Path    string
	Data    []byte
	Version int32
}

type setDataResponse statResponse

type setMaxChildren struct {
	requestHeader
	Path string
	Max  int32
}

type setSaslRequest struct {
	requestHeader
	Token string
}

type setSaslResponse struct {
	responseHeader
	Token string
}

type setWatchesRequest struct {
	requestHeader
	RealtiveZxid int64
	DataWatches  []string
	ExistWatches []string
	ChildWatches []string
}

type setWatchesResponse struct {
	responseHeader
}

type syncRequest pathRequest
type syncResponse pathResponse

type watcherEvent struct {
	responseHeader
	Type  int32
	State int32
	Path  string
}

func decodePacket(buf []byte, st interface{}) (int, error) {
	v := reflect.ValueOf(st)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return 0, errors.New("Invalid ptr")
	}
	return decodePacketValue(buf, v.Elem())
}

func decodePacketValue(buf []byte, v reflect.Value) (int, error) {
	n := 0
	switch v.Kind() {
	default:
		return n, errors.New("Unhandled field type")
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

func encodePacket(buf []byte, st interface{}) (int, error) {
	v := reflect.ValueOf(st)
	if v.Kind() != reflect.Ptr || v.IsNil() {
		return 0, errors.New("Invalid ptr")
	}
	return encodePacketValue(buf, v.Elem())
}

func encodePacketValue(buf []byte, v reflect.Value) (int, error) {
	n := 0
	switch v.Kind() {
	default:
		return n, errors.New("Unhandled field type")
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
