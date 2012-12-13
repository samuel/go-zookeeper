package zk

// TODO: make sure a ping response comes back in a reasonable time

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	bufferSize = 1536 * 1024
)

var (
	ErrConnectionClosed = errors.New("zk: connection closed")
	ErrSessionExpired   = errors.New("zk: session expired")
)

type watcher struct {
	eventType EventType
	ch        chan Event
}

type Conn struct {
	servers        []string
	serverIndex    int
	conn           net.Conn
	state          State
	eventChan      chan Event
	pingInterval   time.Duration
	recvTimeout    time.Duration
	connectTimeout time.Duration

	sendChan     chan *request
	requests     map[int32]*request // Xid -> pending request
	requestsLock sync.Mutex
	watchers     map[string][]*watcher

	xid       int32
	lastZxid  int64
	sessionId int64
	timeout   int32
	passwd    []byte
}

type request struct {
	xid        int32
	opcode     int32
	pkt        interface{}
	recvStruct interface{}
	recvChan   chan error
}

type Event struct {
	Type  EventType
	State State
	Path  string // For non-session events, the path of the watched node.
}

func Connect(servers []string, recvTimeout time.Duration) (*Conn, <-chan Event, error) {
	for i, addr := range servers {
		if !strings.Contains(addr, ":") {
			servers[i] = addr + ":" + strconv.Itoa(defaultPort)
		}
	}
	ec := make(chan Event, 5)
	conn := Conn{
		servers:        servers,
		serverIndex:    0,
		conn:           nil,
		state:          StateDisconnected,
		eventChan:      ec,
		recvTimeout:    recvTimeout,
		pingInterval:   10 * time.Second,
		connectTimeout: 1 * time.Second,
		sendChan:       make(chan *request),
		requests:       make(map[int32]*request),
		watchers:       make(map[string][]*watcher),
		passwd:         emptyPassword,
		timeout:        30000,
	}
	go conn.loop()
	return &conn, ec, nil
}

func (c *Conn) Close() {
	// TODO

	// if c.conn != nil {
	// 	c.conn.Close()
	// }
}

func (c *Conn) connect() {
	startIndex := c.serverIndex
	c.state = StateConnecting
	for {
		zkConn, err := net.DialTimeout("tcp", c.servers[c.serverIndex], c.connectTimeout)
		if err == nil {
			c.conn = zkConn
			c.state = StateConnected
			c.eventChan <- Event{EventSession, c.state, ""}
			return
		}

		log.Printf("Failed to connect to %s: %+v", c.servers[c.serverIndex], err)

		c.serverIndex = (c.serverIndex + 1) % len(c.servers)
		if c.serverIndex == startIndex {
			time.Sleep(time.Second)
		}
	}
}

func (c *Conn) loop() {
	for {
		c.connect()
		err := c.authenticate()
		if err == nil {
			closeChan := make(chan bool)
			go c.sendLoop(c.conn, closeChan)
			err = c.recvLoop(c.conn)
			if err == nil {
				panic("zk: recvLoop should never return nil error")
			}
			close(closeChan)
		}
		c.conn.Close()

		c.state = StateDisconnected
		c.eventChan <- Event{EventSession, c.state, ""}

		log.Println(err)

		c.requestsLock.Lock()
		// Error out any pending requests
		for _, req := range c.requests {
			req.recvChan <- err
		}
		c.requests = make(map[int32]*request)
		c.requestsLock.Unlock()
	}
}

func (c *Conn) authenticate() error {
	buf := make([]byte, 256)

	// connect request

	n, err := encodePacket(buf[4:], &connectRequest{
		ProtocolVersion: protocolVersion,
		LastZxidSeen:    c.lastZxid,
		TimeOut:         c.timeout,
		SessionId:       c.sessionId,
		Passwd:          c.passwd,
	})
	if err != nil {
		return err
	}

	binary.BigEndian.PutUint32(buf[:4], uint32(n))

	_, err = c.conn.Write(buf[:n+4])
	if err != nil {
		return err
	}

	// connect response

	// package length
	_, err = io.ReadFull(c.conn, buf[:4])
	if err != nil {
		return err
	}

	blen := int(binary.BigEndian.Uint32(buf[:4]))
	if cap(buf) < blen {
		buf = make([]byte, blen)
	}

	_, err = io.ReadFull(c.conn, buf[:blen])
	if err != nil {
		return err
	}

	r := connectResponse{}
	_, err = decodePacket(buf[:blen], &r)
	if err != nil {
		return err
	}
	if r.SessionId == 0 {
		c.sessionId = 0
		c.passwd = emptyPassword
		c.state = StateExpired
		c.eventChan <- Event{EventSession, c.state, ""}
		return ErrSessionExpired
	}

	c.timeout = r.TimeOut
	c.sessionId = r.SessionId
	c.passwd = r.Passwd
	c.state = StateHasSession
	// if new session
	c.xid = 0
	c.eventChan <- Event{EventSession, c.state, ""}

	return nil
}

func (c *Conn) sendLoop(conn net.Conn, closeChan <-chan bool) error {
	pingTicker := time.NewTicker(c.pingInterval)
	defer pingTicker.Stop()

	buf := make([]byte, bufferSize)
	for {
		select {
		case req := <-c.sendChan:
			header := &requestHeader{req.xid, req.opcode}
			n, err := encodePacket(buf[4:], header)
			if err != nil {
				req.recvChan <- err
				continue
			}

			n2, err := encodePacket(buf[4+n:], req.pkt)
			if err != nil {
				req.recvChan <- err
				continue
			}

			n += n2

			binary.BigEndian.PutUint32(buf[:4], uint32(n))

			_, err = conn.Write(buf[:n+4])
			if err != nil {
				req.recvChan <- err
				conn.Close()
				return err
			}

			c.requestsLock.Lock()
			select {
			case <-closeChan:
				req.recvChan <- ErrConnectionClosed
				c.requestsLock.Unlock()
				return ErrConnectionClosed
			default:
			}
			c.requests[req.xid] = req
			c.requestsLock.Unlock()
		case <-pingTicker.C:
			n, err := encodePacket(buf[4:], &requestHeader{Xid: -2, Opcode: opPing})
			if err != nil {
				panic("zk: opPing should never fail to serialize")
			}

			binary.BigEndian.PutUint32(buf[:4], uint32(n))

			_, err = conn.Write(buf[:n+4])
			if err != nil {
				conn.Close()
				return err
			}
		case <-closeChan:
			return nil
		}
	}
	panic("not reached")
}

func (c *Conn) recvLoop(conn net.Conn) error {
	buf := make([]byte, bufferSize)
	for {
		// package length
		_, err := io.ReadFull(conn, buf[:4])
		if err != nil {
			return err
		}

		blen := int(binary.BigEndian.Uint32(buf[:4]))
		if cap(buf) < blen {
			buf = make([]byte, blen)
		}

		_, err = io.ReadFull(conn, buf[:blen])
		if err != nil {
			return err
		}

		res := responseHeader{}
		_, err = decodePacket(buf[:16], &res)
		if err != nil {
			return err
		}

		// log.Printf("Response xid=%d zxid=%d err=%d\n", res.Xid, res.Zxid, res.Err)

		if res.Xid == -1 {
			res := &watcherEvent{}
			_, err := decodePacket(buf[16:16+blen], res)
			if err != nil {
				return err
			}
			ev := Event{
				Type:  res.Type,
				State: res.State,
				Path:  res.Path,
			}
			c.eventChan <- ev
			if watchers := c.watchers[res.Path]; watchers != nil {
				for _, w := range watchers {
					if w.eventType == res.Type {
						w.ch <- ev
					}
				}
			}
		} else if res.Xid == -2 {
			// Ping response. Ignore.
		} else if res.Xid < 0 {
			log.Printf("Xid < 0 (%d) but not ping or watcher event", res.Xid)
		} else {
			if res.Zxid > 0 {
				c.lastZxid = res.Zxid
			}

			c.requestsLock.Lock()
			req, ok := c.requests[res.Xid]
			if ok {
				delete(c.requests, res.Xid)
			}
			c.requestsLock.Unlock()

			if !ok {
				log.Printf("Response for unknown request with xid %d", res.Xid)
			} else {
				if res.Err != 0 {
					err = res.Err.toError()
				} else {
					_, err = decodePacket(buf[16:16+blen], req.recvStruct)
				}
				req.recvChan <- err
			}
		}
	}
	panic("not reached")
}

func (c *Conn) nextXid() int32 {
	return atomic.AddInt32(&c.xid, 1)
}

func (c *Conn) request(opcode int32, req interface{}, res interface{}) error {
	ch := make(chan error)
	rq := &request{
		xid:        c.nextXid(),
		opcode:     opcode,
		pkt:        req,
		recvStruct: res,
		recvChan:   ch,
	}
	c.sendChan <- rq
	return <-ch
}

func (c *Conn) Children(path string) ([]string, *Stat, error) {
	res := &getChildren2Response{}
	err := c.request(opGetChildren2, &getChildren2Request{Path: path, Watch: false}, res)
	return res.Children, &res.Stat, err
}

func (c *Conn) ChildrenW(path string) ([]string, *Stat, chan Event, error) {
	res := &getChildren2Response{}
	err := c.request(opGetChildren2, &getChildren2Request{Path: path, Watch: true}, res)

	// TODO: This is totally borked. Not safe.. not reliable..
	var ech chan Event
	if err == nil {
		ech = make(chan Event, 1)
		watchers := c.watchers[path]
		if watchers == nil {
			watchers = make([]*watcher, 0)
		}
		c.watchers[path] = append(watchers, &watcher{EventNodeChildrenChanged, ech})
	}
	return res.Children, &res.Stat, ech, err
}

func (c *Conn) Get(path string) ([]byte, *Stat, error) {
	res := &getDataResponse{}
	err := c.request(opGetData, &getDataRequest{Path: path, Watch: false}, res)
	return res.Data, &res.Stat, err
}

func (c *Conn) Set(path string, data []byte, version int32) (*Stat, error) {
	res := &setDataResponse{}
	err := c.request(opSetData, &setDataRequest{path, data, version}, res)
	return &res.Stat, err
}

func (c *Conn) Create(path string, data []byte, flags int32, acl []ACL) (string, error) {
	res := &createResponse{}
	err := c.request(opCreate, &createRequest{path, data, acl, flags}, res)
	return res.Path, err
}

func (c *Conn) Delete(path string, version int32) error {
	res := &deleteResponse{}
	return c.request(opDelete, &deleteRequest{path, version}, res)
}
