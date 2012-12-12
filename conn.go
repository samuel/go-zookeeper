package zk

// TODO: make sure a ping response comes back in a reasonable time

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"net"
	// "os"
	"fmt"
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
	closeChan    chan bool
	watchers     map[string][]*watcher

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
}

func (c *Conn) connect() error {
	startIndex := c.serverIndex
	c.state = StateConnecting
	count := 0
	for {
		zkConn, err := net.DialTimeout("tcp", c.servers[c.serverIndex], c.connectTimeout)
		if err == nil {
			c.conn = zkConn
			c.state = StateConnected
			c.closeChan = make(chan bool)
			c.eventChan <- Event{EventSession, c.state, ""}
			return nil
		}
		// disabled to stop noise, after 5 failures this error with percolate up
		// log.Printf("zk: Failed to connect to %s: %+v", c.servers[c.serverIndex], err)
		if count > 5 {
			return errors.New("zk: Can't connect")
		}
		count++

		c.serverIndex = (c.serverIndex + 1) % len(c.servers)
		if c.serverIndex == startIndex {
			time.Sleep(time.Second)
		}
	}
	return errors.New("zk: Can't connect 2")
}

func (c *Conn) loop() {
	for {
		c.connect()
		err := c.handler()
		if err == nil {
			panic("zk: handler should never return nil error")
		}
		c.conn.Close()
		close(c.closeChan)

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

func (c *Conn) handler() (e error) {
	defer func() {
		if r := recover(); r != nil {
			e = errors.New("UNKWNREQ: handler: " + fmt.Sprintf("%v", r))
			// fmt.Println("Recovered in f", r)
		}
	}()

	buf := make([]byte, 1024)

	// connect request
	re := connectRequest{
		ProtocolVersion: protocolVersion,
		LastZxidSeen:    c.lastZxid,
		TimeOut:         c.timeout,
		SessionId:       c.sessionId,
		Passwd:          c.passwd,
	}
	n, err := encodePacket(buf[4:], &re)
	// log.Printf(">>>>>>>>>>>>>> handler n is %+v\n", n)
	// log.Printf(">>>>>>>>>>>>>> re before is %+v\n", re)
	// log.Printf(">>>>>>>>>>>>>> buf is %+v\n", buf)
	if err != nil {
		return err
	}

	binary.BigEndian.PutUint32(buf[:4], uint32(n))

	// log.Printf(">>>>>>>>>>>>>> re after is %+v\n", re)
	// log.Printf(">>>>>>>>>>>>>> buf after %+v\n", buf)
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
	// log.Printf(">>>>>>>>>>>>>> r is %+v\n", r)
	// log.Printf(">>>>>>>>>>>>>> buf[:blen] is %+v\n", buf[:blen])

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

	// Send loop
	go func() {
		// log.Println("SENDING REQUEST TO ZK---------------------------------------------------------")
		closeChan := c.closeChan
		pingTicker := time.NewTicker(c.pingInterval)
		defer pingTicker.Stop()
		buf := make([]byte, bufferSize)
		for {
			select {
			case req := <-c.sendChan:
				// log.Printf("--------- ++++++ -----> c.sendChan req is =>%+v\n", req)
				// log.Printf("--------- ++++++ -----> req =>%+v\n", req)
				// log.Printf("--------- ++++++ -----> req.pkt =>%+v\n", req.pkt)
				n, err := encodePacket(buf[4:], req.pkt)
				if err != nil {
					req.recvChan <- err
					continue
				}

				binary.BigEndian.PutUint32(buf[:4], uint32(n))

				// these are the ones
				// log.Printf("--------- +||||+ -----> req =>%+v\n", req)
				// log.Printf("--------- +||||+ -----> req.pkt =>%+v\n", req.pkt)
				// log.Printf("--------- +||||+ -----> n =>%+v\n", n)
				// log.Printf("--------- +||||+ -----> buf =>%+v\n", buf)

				_, err = c.conn.Write(buf[:n+4])
				if err != nil {
					req.recvChan <- err
					c.conn.Close()
					return
				}

				c.requestsLock.Lock()
				select {
				case <-closeChan:
					req.recvChan <- ErrConnectionClosed
					c.requestsLock.Unlock()
					return
				default:
				}
				c.requests[req.xid] = req
				// log.Printf("--------- ++++++ -----> req is =>%+v\n", req)
				// log.Printf("--------- ++++++ -----> req.pkt is =>%+v\n", req.pkt)
				// log.Printf("--------- ++++++ -----> c.requests is =>%+v\n", c.requests)
				c.requestsLock.Unlock()
			case <-pingTicker.C:
				n, err := encodePacket(buf[4:], &pingRequest{requestHeader{Xid: -2, Opcode: opPing}})
				if err != nil {
					panic("zk: opPing should never fail to serialize")
				}

				binary.BigEndian.PutUint32(buf[:4], uint32(n))

				_, err = c.conn.Write(buf[:n+4])
				if err != nil {
					c.conn.Close()
					return
				}
			case <-closeChan:
				return
			}
		}
	}()

	// Receive loop
	for {
		// log.Println("READING RESPONSE FROM ZK------------------------------------------------------")
		// package length
		// log.Printf("--------------< buf =>%+v\n", buf)
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

		res := responseHeader{}
		_, err = decodePacket(buf[:16], &res)
		if err != nil {
			return err
		}

		// log.Printf("READING RESPONSE HEADER %+v\n: ", res)
		// log.Printf("Response xid=%d zxid=%d err=%d\n", res.Xid, res.Zxid, res.Err)

		if res.Zxid > 0 {
			c.lastZxid = res.Zxid
		}

		if res.Xid == -1 {
			res := &watcherEvent{}
			_, err := decodePacket(buf[:blen], res)
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
		} else {
			c.requestsLock.Lock()
			// log.Println("in res.Xid default")
			// res.Xid = -1 // testing
			// log.Println(c.requests)
			req, ok := c.requests[res.Xid]
			if ok {
				delete(c.requests, res.Xid)
			}
			c.requestsLock.Unlock()

			if !ok {
				log.Printf("Response for unknown request with xid %d", res.Xid)
				req.recvChan <- errors.New("UNKWNREQ: Response for unknown request with xid " + string(res.Xid))
			} else {
				_, err := decodePacket(buf[:blen], req.recvStruct)
				req.recvChan <- err
			}
		}
	}

	// Shouldn't ever get here
	panic("zk: handler should never reach the end")
}

func (c *Conn) nextXid() int32 {
	return atomic.AddInt32(&c.xid, 1)
}

func (c *Conn) Set(path string, data []byte) (stat *Stat, err error) {
	// log.Println("---> in Set")
	// log.Println(string(data))
	xid := c.nextXid()
	ch := make(chan error)
	rs := &setDataResponse{}
	req := &request{
		xid: xid,
		pkt: &setDataRequest{
			requestHeader: requestHeader{
				Xid:    xid,
				Opcode: opSetData,
			},
			Path:    path,
			Data:    data,
			Version: -1,
		},
		recvStruct: rs,
		recvChan:   ch,
	}
	// log.Println("----------set request------------------------")
	// log.Printf("%+v\n", req)
	// log.Printf("%+v\n", req.pkt)
	c.sendChan <- req
	err = <-ch
	stat = &rs.Stat
	// log.Println("----------set response------------------------")
	// log.Printf("%+v\n", rs)
	// log.Printf("err:%+v\n", err)
	// log.Printf("stat:%+v\n", *stat)
	// log.Println("<--- out Set")
	return
}

func (c *Conn) Create(path string, data []byte) (rpath *string, err error) {
	log.Println("---> in Create")
	log.Println(string(data))
	xid := c.nextXid()
	ch := make(chan error)
	rs := &createResponse{}
	req := &request{
		xid: xid,
		pkt: &createRequest{
			requestHeader: requestHeader{
				Xid:    xid,
				Opcode: opCreate,
			},
			Path:  path,
			Data:  data,
			Acl:   []acl{acl{0x1f, id{"world", 0}}},
			Flags: 0,
		},
		recvStruct: rs,
		recvChan:   ch,
	}
	log.Println("----------create request------------------------")
	log.Printf("%+v\n", req)
	log.Printf("%+v\n", req.pkt)
	c.sendChan <- req
	err = <-ch
	rpath = &rs.Path
	log.Println("----------create response------------------------")
	log.Printf("%+v\n", rs)
	log.Printf("err:%+v\n", err)
	log.Printf("rpath:%+v\n", *rpath)
	log.Println("<--- out Create")
	return
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

func (c *Conn) ChildrenW(path string) ([]string, *Stat, chan Event, error) {
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
	err := <-ch
	var ech chan Event
	if err == nil {
		ech = make(chan Event, 1)
		watchers := c.watchers[path]
		if watchers == nil {
			watchers = make([]*watcher, 0)
		}
		c.watchers[path] = append(watchers, &watcher{EventNodeChildrenChanged, ech})

	}
	return rs.Children, &rs.Stat, ech, err
}

func (c *Conn) Get(path string) (data []byte, stat *Stat, err error) {
	// log.Println("---> in Get")
	// log.Println(string(data))
	xid := c.nextXid()
	ch := make(chan error)
	rs := &getDataResponse{}
	req := &request{
		xid: xid,
		pkt: &getDataRequest{
			requestHeader: requestHeader{
				Xid:    xid,
				Opcode: opGetData,
			},
			Path:  path,
			Watch: false,
		},
		recvStruct: rs,
		recvChan:   ch,
	}
	// log.Println("----------get request------------------------")
	// log.Printf("%+v\n", req)
	// log.Printf("%+v\n", req.pkt)
	c.sendChan <- req
	err = <-ch
	data = rs.Data
	stat = &rs.Stat
	// log.Println("----------get response------------------------")
	// log.Printf("%+v\n", rs)
	// log.Printf("err:%+v\n", err)
	// log.Printf("stat:%+v\n", *stat)
	// log.Println("<--- out Get")
	return
}
