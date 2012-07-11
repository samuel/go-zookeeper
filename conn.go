package zk

// TODO: ping server
// TODO: everything

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type state int

type Conn struct {
	servers        []string
	serverIndex    int
	conn           net.Conn
	state          state
	eventChan      <-chan Event
	pingInterval   time.Duration
	recvTimeout    time.Duration
	connectTimeout time.Duration

	sendChan     chan *request
	requests     map[int32]*request // Xid -> pending request
	requestsLock sync.Mutex

	xid       int32
	lastZxid  int64
	sessionId int64
	timeout   int32
	passwd    []byte
}

type request struct {
	xid        int32
	pkt        interface{}
	recvStruct interface{}
	recvChan   chan error
}

type Event struct {
	Type  EventType
	State int    // One of the STATE_* constants.
	Path  string // For non-session events, the path of the watched node.
}

func Connect(servers []string, recvTimeout time.Duration) (*Conn, <-chan Event, error) {
	ec := make(<-chan Event, 5)
	conn := Conn{
		servers:        servers,
		serverIndex:    0,
		conn:           nil,
		state:          stateDisconnected,
		eventChan:      ec,
		recvTimeout:    recvTimeout,
		pingInterval:   10 * time.Second,
		connectTimeout: 1 * time.Second,
		sendChan:       make(chan *request),
		requests:       make(map[int32]*request),
	}
	go conn.loop()
	return &conn, ec, nil
}

func (c *Conn) connect() {
	startIndex := c.serverIndex
	for {
		zkConn, err := net.DialTimeout("tcp", c.servers[c.serverIndex], c.connectTimeout)
		if err == nil {
			c.conn = zkConn
			c.state = stateConnected
			return
		}
		c.serverIndex = (c.serverIndex + 1) % len(c.servers)
		if c.serverIndex == startIndex {
			time.Sleep(time.Second)
		}
	}
}

func (c *Conn) loop() {
	for {
		if c.conn != nil {
			c.conn.Close()
		}
		c.conn = nil
		c.state = stateDisconnected

		c.connect()
		c.handler()
	}
}

func (c *Conn) handler() {
	buf := make([]byte, 1024)

	// connect request

	n, err := encodePacket(buf[4:], &connectRequest{
		ProtocolVersion: protocolVersion,
		LastZxidSeen:    c.lastZxid,
		TimeOut:         30000,
		SessionId:       c.sessionId,
		Passwd:          emptyPassword,
	})
	if err != nil {
		log.Println(err)
		return
	}

	binary.BigEndian.PutUint32(buf[:4], uint32(n))

	_, err = c.conn.Write(buf[:n+4])
	if err != nil {
		log.Println(err)
		return
	}

	// connect response

	// package length
	_, err = io.ReadFull(c.conn, buf[:4])
	if err != nil {
		log.Println(err)
		return
	}

	blen := int(binary.BigEndian.Uint32(buf[:4]))
	if cap(buf) < blen {
		buf = make([]byte, blen)
	}

	_, err = io.ReadFull(c.conn, buf[:blen])
	if err != nil {
		log.Println(err)
		return
	}

	r := connectResponse{}
	decodePacket(buf[:blen], r)
	// TODO: Check for a dead session
	c.timeout = r.TimeOut
	c.sessionId = r.SessionId
	c.passwd = r.Passwd
	c.state = stateHasSession
	// if new session
	c.xid = 0

	// Send loop
	go func() {
		buf := make([]byte, 1024)
		for {
			req := <-c.sendChan
			n, err := encodePacket(buf[4:], req.pkt)
			if err != nil {
				req.recvChan <- err
				continue
			}
			binary.BigEndian.PutUint32(buf[:4], uint32(n))

			c.requestsLock.Lock()
			c.requests[req.xid] = req
			c.requestsLock.Unlock()

			_, err = c.conn.Write(buf[:n+4])
			if err != nil {
				req.recvChan <- err
				return
			}
		}
	}()

	// Receive loop
	for {
		// package length
		_, err = io.ReadFull(c.conn, buf[:4])
		if err != nil {
			log.Println(err)
			return
		}

		blen := int(binary.BigEndian.Uint32(buf[:4]))
		if cap(buf) < blen {
			buf = make([]byte, blen)
		}

		_, err = io.ReadFull(c.conn, buf[:blen])
		if err != nil {
			log.Println(err)
			return
		}

		res := responseHeader{}
		_, err := decodePacket(buf[:16], &res)
		if err != nil {
			// TODO
			panic(err)
		}

		log.Printf("Response xid=%d zxid=%d err=%d\n", res.Xid, res.Zxid, res.Err)

		if res.Zxid > 0 {
			c.lastZxid = res.Zxid
		}

		if res.Xid == -1 {
			res := &watcherEvent{}
			_, err := decodePacket(buf[:blen], res)
			if err != nil {
				panic(err)
			}
			log.Printf("%+v\n", res)
		} else {
			c.requestsLock.Lock()
			req, ok := c.requests[res.Xid]
			if ok {
				delete(c.requests, res.Xid)
			}
			c.requestsLock.Unlock()

			if !ok {
				log.Printf("Response for unknown request with xid %d", res.Xid)
			} else {
				_, err := decodePacket(buf[:blen], req.recvStruct)
				req.recvChan <- err
			}
		}
	}
}

func (c *Conn) nextXid() int32 {
	return atomic.AddInt32(&c.xid, 1)
}

func (c *Conn) Children(path string) (children []string, stat *Stat, err error) {
	xid := c.nextXid()
	ch := make(chan error)
	rs := &getChildren2Response{}
	req := &request{
		xid: xid,
		pkt: &getChildren2Request{
			requestHeader: requestHeader{
				Xid:    xid,
				Opcode: opGetChildren2,
			},
			Path:  path,
			Watch: false,
		},
		recvStruct: rs,
		recvChan:   ch,
	}
	c.sendChan <- req
	err = <-ch
	children = rs.Children
	stat = &rs.Stat
	return
}

func (c *Conn) ChildrenW(path string) (children []string, stat *Stat, err error) {
	xid := c.nextXid()
	ch := make(chan error)
	rs := &getChildren2Response{}
	req := &request{
		xid: xid,
		pkt: &getChildren2Request{
			requestHeader: requestHeader{
				Xid:    xid,
				Opcode: opGetChildren2,
			},
			Path:  path,
			Watch: true,
		},
		recvStruct: rs,
		recvChan:   ch,
	}
	c.sendChan <- req
	err = <-ch
	children = rs.Children
	stat = &rs.Stat
	return
}
