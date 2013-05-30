package zk

// TODO: make sure a ping response comes back in a reasonable time

import (
	"encoding/binary"
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
	bufferSize    = 1536 * 1024
	eventChanSize = 5
	sendChanSize  = 16
)

type watcherType int

var (
	watcherTypeData  = watcherType(1)
	watcherTypeExist = watcherType(2)
	watcherTypeChild = watcherType(3)
)

func (t watcherType) matchesEvent(e EventType) bool {
	if e == EventNodeCreated && t == watcherTypeExist {
		return true
	}
	if (e == EventNodeDeleted || e == EventNodeDataChanged) && (t == watcherTypeExist || t == watcherTypeData) {
		return true
	}
	return e == EventNodeChildrenChanged && t == watcherTypeChild
}

type watchers struct {
	dataWatchers  []chan Event
	existWatchers []chan Event
	childWatchers []chan Event
}

type Conn struct {
	servers        []string
	serverIndex    int
	conn           net.Conn
	state          State
	eventChan      chan Event
	shouldQuit     chan bool
	pingInterval   time.Duration
	recvTimeout    time.Duration
	connectTimeout time.Duration

	sendChan     chan *request
	requests     map[int32]*request // Xid -> pending request
	requestsLock sync.Mutex
	watchers     map[string]*watchers
	watchersLock sync.Mutex

	xid       int32
	lastZxid  int64
	sessionId int64
	timeout   int32
	passwd    []byte

	// Debug (used by unit tests)
	reconnectDelay time.Duration
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
	Err   error
}

func Connect(servers []string, recvTimeout time.Duration) (*Conn, <-chan Event, error) {
	for i, addr := range servers {
		if !strings.Contains(addr, ":") {
			servers[i] = addr + ":" + strconv.Itoa(defaultPort)
		}
	}
	ec := make(chan Event, eventChanSize)
	conn := Conn{
		servers:        servers,
		serverIndex:    0,
		conn:           nil,
		state:          StateDisconnected,
		eventChan:      ec,
		shouldQuit:     make(chan bool),
		recvTimeout:    recvTimeout,
		pingInterval:   10 * time.Second,
		connectTimeout: 1 * time.Second,
		sendChan:       make(chan *request, sendChanSize),
		requests:       make(map[int32]*request),
		watchers:       make(map[string]*watchers),
		passwd:         emptyPassword,
		timeout:        30000,

		// Debug
		reconnectDelay: 0,
	}
	go func() {
		conn.loop()
		conn.flushRequests(ErrConnectionClosed)
		conn.invalidateWatches(ErrConnectionClosed)
	}()
	return &conn, ec, nil
}

func (c *Conn) Close() {
	close(c.shouldQuit)
	c.disconnect()
}

func (c *Conn) connect() {
	startIndex := c.serverIndex
	c.state = StateConnecting
	for {
		zkConn, err := net.DialTimeout("tcp", c.servers[c.serverIndex], c.connectTimeout)
		if err == nil {
			c.conn = zkConn
			c.state = StateConnected
			c.eventChan <- Event{EventSession, c.state, "", nil}
			return
		}

		log.Printf("Failed to connect to %s: %+v", c.servers[c.serverIndex], err)

		c.serverIndex = (c.serverIndex + 1) % len(c.servers)
		if c.serverIndex == startIndex {
			time.Sleep(time.Second)
		}
	}
}

func (c *Conn) disconnect() {
	c.queueRequest(-1, nil, nil, make(chan error, 1))
}

func (c *Conn) loop() {
	for {
		c.connect()
		err := c.authenticate()
		if err == ErrSessionExpired {
			c.invalidateWatches(err)
		} else if err == nil {
			closeChan := make(chan bool)
			sendDone := make(chan bool, 1)
			go func() {
				c.sendLoop(c.conn, closeChan)
				c.conn.Close()
				close(sendDone)
			}()

			recvDone := make(chan bool, 1)
			go func() {
				err = c.recvLoop(c.conn)
				if err == nil {
					panic("zk: recvLoop should never return nil error")
				}
				close(closeChan)
				<-sendDone // wait for send loop to exit
				close(recvDone)
			}()

			<-recvDone
		}

		c.state = StateDisconnected
		c.eventChan <- Event{EventSession, c.state, "", nil}

		// Yeesh
		if !strings.Contains(err.Error(), "use of closed network connection") {
			log.Println(err)
		}

		c.flushRequests(err)

		if c.reconnectDelay > 0 {
			select {
			case <-c.shouldQuit:
				return
			case <-time.After(c.reconnectDelay):
			}
		} else {
			select {
			case <-c.shouldQuit:
				return
			default:
			}
		}
	}
}

func (c *Conn) flushRequests(err error) {
	c.requestsLock.Lock()
	// Error out any pending requests
	for _, req := range c.requests {
		req.recvChan <- err
	}
	c.requests = make(map[int32]*request)
	c.requestsLock.Unlock()
}

func (c *Conn) invalidateWatches(err error) {
	c.watchersLock.Lock()
	defer c.watchersLock.Unlock()

	if len(c.watchers) >= 0 {
		for path, wat := range c.watchers {
			ev := Event{EventSession, c.state, path, err}
			for _, ch := range wat.dataWatchers {
				ch <- ev
			}
			for _, ch := range wat.childWatchers {
				ch <- ev
			}
			for _, ch := range wat.existWatchers {
				ch <- ev
			}
		}
		c.watchers = make(map[string]*watchers, 0)
	}
}

func (c *Conn) sendSetWatches() {
	c.watchersLock.Lock()
	defer c.watchersLock.Unlock()

	if len(c.watchers) == 0 {
		return
	}

	req := &setWatchesRequest{
		RealtiveZxid: c.lastZxid,
		DataWatches:  make([]string, 0),
		ExistWatches: make([]string, 0),
		ChildWatches: make([]string, 0),
	}
	pathLen := 0
	for path, watchers := range c.watchers {
		if len(watchers.dataWatchers) != 0 {
			req.DataWatches = append(req.DataWatches, path)
			pathLen += len(path)
		}
		if len(watchers.existWatchers) != 0 {
			req.ExistWatches = append(req.ExistWatches, path)
			pathLen += len(path)
		}
		if len(watchers.childWatchers) != 0 {
			req.ChildWatches = append(req.ChildWatches, path)
			pathLen += len(path)
		}
	}
	if pathLen == 0 {
		return
	}

	go func() {
		res := &setWatchesResponse{}
		err := c.request(opSetWatches, req, res)
		if err != nil {
			log.Fatal(err)
		}
	}()
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

	c.sendSetWatches()

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
		c.eventChan <- Event{EventSession, c.state, "", nil}
		return ErrSessionExpired
	}

	if c.sessionId != r.SessionId {
		c.xid = 0
	}
	c.timeout = r.TimeOut
	c.sessionId = r.SessionId
	c.passwd = r.Passwd
	c.state = StateHasSession
	c.eventChan <- Event{EventSession, c.state, "", nil}

	return nil
}

func (c *Conn) sendLoop(conn net.Conn, closeChan <-chan bool) error {
	pingTicker := time.NewTicker(c.pingInterval)
	defer pingTicker.Stop()

	buf := make([]byte, bufferSize)
	for {
		select {
		case req := <-c.sendChan:
			if req.opcode < 0 {
				// Asked to quit
				req.recvChan <- nil
				return nil
			}

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
				Err:   nil,
			}
			select {
			case c.eventChan <- ev:
			default:
			}
			c.watchersLock.Lock()
			if wat := c.watchers[res.Path]; wat != nil {
				switch res.Type {
				case EventNodeCreated:
					for _, ch := range wat.existWatchers {
						ch <- ev
					}
					wat.existWatchers = wat.existWatchers[:0]
				case EventNodeDeleted, EventNodeDataChanged:
					for _, ch := range wat.existWatchers {
						ch <- ev
					}
					for _, ch := range wat.dataWatchers {
						ch <- ev
					}
					wat.existWatchers = wat.existWatchers[:0]
					wat.dataWatchers = wat.dataWatchers[:0]
				case EventNodeChildrenChanged:
					for _, ch := range wat.childWatchers {
						ch <- ev
					}
					wat.childWatchers = wat.childWatchers[:0]
				}
				if len(wat.childWatchers)+len(wat.dataWatchers)+len(wat.existWatchers) == 0 {
					delete(c.watchers, res.Path)
				}
			}
			c.watchersLock.Unlock()
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

func (c *Conn) addWatcher(path string, watcherType watcherType) chan Event {
	c.watchersLock.Lock()
	defer c.watchersLock.Unlock()

	ch := make(chan Event, 1)
	wat := c.watchers[path]
	if wat == nil {
		wat = &watchers{
			dataWatchers:  make([]chan Event, 0),
			existWatchers: make([]chan Event, 0),
			childWatchers: make([]chan Event, 0),
		}
		c.watchers[path] = wat
	}
	switch watcherType {
	case watcherTypeChild:
		wat.childWatchers = append(wat.childWatchers, ch)
	case watcherTypeData:
		wat.dataWatchers = append(wat.dataWatchers, ch)
	case watcherTypeExist:
		wat.existWatchers = append(wat.existWatchers, ch)
	}
	return ch
}

func (c *Conn) queueRequest(opcode int32, req interface{}, res interface{}, errChan chan error) {
	rq := &request{
		xid:        c.nextXid(),
		opcode:     opcode,
		pkt:        req,
		recvStruct: res,
		recvChan:   errChan,
	}
	c.sendChan <- rq
}

func (c *Conn) request(opcode int32, req interface{}, res interface{}) error {
	ch := make(chan error)
	c.queueRequest(opcode, req, res, ch)
	return <-ch
}

func (c *Conn) Children(path string) ([]string, *Stat, error) {
	res := &getChildren2Response{}
	err := c.request(opGetChildren2, &getChildren2Request{Path: path, Watch: false}, res)
	return res.Children, &res.Stat, err
}

func (c *Conn) ChildrenW(path string) ([]string, *Stat, <-chan Event, error) {
	res := &getChildren2Response{}
	err := c.request(opGetChildren2, &getChildren2Request{Path: path, Watch: true}, res)
	var ech chan Event
	if err == nil {
		ech = c.addWatcher(path, watcherTypeChild)
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

func (c *Conn) ExistsW(path string) (bool, *Stat, <-chan Event, error) {
	res := &existsResponse{}
	err := c.request(opExists, &existsRequest{Path: path, Watch: true}, res)
	exists := true
	if err == ErrNoNode {
		exists = false
		err = nil
	}
	var ech chan Event
	if err == nil {
		if exists {
			ech = c.addWatcher(path, watcherTypeData)
		} else {
			ech = c.addWatcher(path, watcherTypeExist)
		}
	}
	return exists, &res.Stat, ech, err
}
