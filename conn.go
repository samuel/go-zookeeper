package zk

/*
TODO:
* make sure a ping response comes back in a reasonable time
* timeouts on network calls
*/

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
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
	bufferSize      = 1536 * 1024
	eventChanSize   = 6
	sendChanSize    = 16
	protectedPrefix = "_c_"
)

type watcherType int

var (
	watcherTypeData  = watcherType(1)
	watcherTypeExist = watcherType(2)
	watcherTypeChild = watcherType(3)
)

type watcher struct {
	ch   chan Event
	zxid int64
}

type watchers struct {
	dataWatchers  []*watcher
	existWatchers []*watcher
	childWatchers []*watcher
}

type Conn struct {
	lastZxid  int64
	sessionId int64
	state     State // must be 32-bit aligned
	xid       int32
	timeout   int32 // session timeout in seconds
	passwd    []byte

	servers        []string
	serverIndex    int
	conn           net.Conn
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

	// Debug (used by unit tests)
	reconnectDelay time.Duration
}

type request struct {
	xid        int32
	opcode     int32
	pkt        interface{}
	recvStruct interface{}
	recvChan   chan response
}

type response struct {
	zxid int64
	err  error
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
		conn.flushRequests(ErrClosing)
		conn.invalidateWatches(ErrClosing)
	}()
	return &conn, ec, nil
}

func (c *Conn) Close() {
	close(c.shouldQuit)

	select {
	case <-c.queueRequest(opClose, &closeRequest{}, &closeResponse{}):
	case <-time.After(time.Second):
	}
}

func (c *Conn) State() State {
	return State(atomic.LoadInt32((*int32)(&c.state)))
}

func (c *Conn) setState(state State) {
	atomic.StoreInt32((*int32)(&c.state), int32(state))
	select {
	case c.eventChan <- Event{Type: EventSession, State: state}:
	default:
		// panic("zk: event channel full - it must be monitored and never allowed to be full")
	}
}

func (c *Conn) connect() {
	startIndex := c.serverIndex
	c.setState(StateConnecting)
	for {
		zkConn, err := net.DialTimeout("tcp", c.servers[c.serverIndex], c.connectTimeout)
		if err == nil {
			c.conn = zkConn
			c.setState(StateConnected)
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
		if err == ErrSessionExpired {
			c.invalidateWatches(err)
		} else if err == nil {
			closeChan := make(chan bool) // channel to tell send loop stop

			sendDone := make(chan bool, 1) // channel signaling that send loop is done
			go func() {
				c.sendLoop(c.conn, closeChan)
				c.conn.Close()  // causes recv loop to EOF/exit
				close(sendDone) // tell recv loop we're done
			}()

			recvDone := make(chan bool, 1) // channel signaling that recv loop is done
			go func() {
				err = c.recvLoop(c.conn)
				if err == nil {
					panic("zk: recvLoop should never return nil error")
				}
				close(closeChan) // tell send loop to exit
				<-sendDone       // wait for send loop to exit
				close(recvDone)  // allow main loop to continue
			}()

			<-recvDone // wait for recv loop to finish which waits for the send loop

			// At this point both send and receive loops have stopped, and the
			// socket should be closed.
		}

		c.setState(StateDisconnected)

		// Yeesh
		if err != io.EOF && err != ErrSessionExpired && !strings.Contains(err.Error(), "use of closed network connection") {
			log.Println(err)
		}

		if err != ErrSessionExpired {
			err = ErrConnectionClosed
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

// Send error to all pending requests and clear request map
func (c *Conn) flushRequests(err error) {
	c.requestsLock.Lock()
	for _, req := range c.requests {
		req.recvChan <- response{-1, err}
	}
	c.requests = make(map[int32]*request)
	c.requestsLock.Unlock()
}

// Send error to all watchers and clear watchers map
func (c *Conn) invalidateWatches(err error) {
	c.watchersLock.Lock()
	defer c.watchersLock.Unlock()

	if len(c.watchers) >= 0 {
		for path, wat := range c.watchers {
			ev := Event{Type: EventNotWatching, State: StateDisconnected, Path: path, Err: err}
			for _, w := range wat.dataWatchers {
				w.ch <- ev
			}
			for _, w := range wat.childWatchers {
				w.ch <- ev
			}
			for _, w := range wat.existWatchers {
				w.ch <- ev
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
		RelativeZxid: c.lastZxid,
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
		_, err := c.request(opSetWatches, req, res)
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
		c.setState(StateExpired)
		return ErrSessionExpired
	}

	if c.sessionId != r.SessionId {
		c.xid = 0
	}
	c.timeout = r.TimeOut
	c.sessionId = r.SessionId
	c.passwd = r.Passwd
	c.setState(StateHasSession)

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
				req.recvChan <- response{-1, err}
				continue
			}

			n2, err := encodePacket(buf[4+n:], req.pkt)
			if err != nil {
				req.recvChan <- response{-1, err}
				continue
			}

			n += n2

			binary.BigEndian.PutUint32(buf[:4], uint32(n))

			c.requestsLock.Lock()
			select {
			case <-closeChan:
				req.recvChan <- response{-1, ErrConnectionClosed}
				c.requestsLock.Unlock()
				return ErrConnectionClosed
			default:
			}
			c.requests[req.xid] = req
			c.requestsLock.Unlock()

			_, err = conn.Write(buf[:n+4])
			if err != nil {
				req.recvChan <- response{-1, err}
				conn.Close()
				return err
			}
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

func signalWatchers(watchers []*watcher, zxid int64, ev Event) []*watcher {
	n := len(watchers)
	for i := 0; i < n; {
		w := watchers[i]
		if w.zxid > 0 && w.zxid <= zxid {
			watchers[i] = watchers[n-1]
			n--
			w.ch <- ev
		} else {
			i++
		}
	}
	return watchers[:n]
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
				zxid := c.lastZxid
				switch res.Type {
				case EventNodeCreated:
					wat.existWatchers = signalWatchers(wat.existWatchers, zxid, ev)
				case EventNodeDeleted, EventNodeDataChanged:
					wat.existWatchers = signalWatchers(wat.existWatchers, zxid, ev)
					wat.dataWatchers = signalWatchers(wat.dataWatchers, zxid, ev)
				case EventNodeChildrenChanged:
					wat.childWatchers = signalWatchers(wat.childWatchers, zxid, ev)
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
				req.recvChan <- response{res.Zxid, err}
				if req.opcode == opClose {
					return io.EOF
				}
			}
		}
	}
	panic("not reached")
}

func (c *Conn) nextXid() int32 {
	return atomic.AddInt32(&c.xid, 1)
}

func (c *Conn) addWatcher(path string, watcherType watcherType, zxid int64) *watcher {
	c.watchersLock.Lock()
	defer c.watchersLock.Unlock()

	ch := make(chan Event, 1)
	wat := c.watchers[path]
	if wat == nil {
		wat = &watchers{
			dataWatchers:  make([]*watcher, 0),
			existWatchers: make([]*watcher, 0),
			childWatchers: make([]*watcher, 0),
		}
		c.watchers[path] = wat
	}
	w := &watcher{ch, zxid}
	switch watcherType {
	case watcherTypeChild:
		wat.childWatchers = append(wat.childWatchers, w)
	case watcherTypeData:
		wat.dataWatchers = append(wat.dataWatchers, w)
	case watcherTypeExist:
		wat.existWatchers = append(wat.existWatchers, w)
	}
	return w
}

func removeWatcherFromList(watchers []*watcher, w *watcher) ([]*watcher, bool) {
	for i := 0; i < len(watchers); i++ {
		if watchers[i] == w {
			watchers[i] = watchers[len(watchers)-1]
			return watchers[:len(watchers)-1], true
		}
	}
	return watchers, false
}

func (c *Conn) removeWatcher(path string, watcherType watcherType, w *watcher) bool {
	c.watchersLock.Lock()
	defer c.watchersLock.Unlock()

	wat := c.watchers[path]
	if wat == nil {
		return false
	}

	found := false
	switch watcherType {
	case watcherTypeChild:
		wat.childWatchers, found = removeWatcherFromList(wat.childWatchers, w)
	case watcherTypeData:
		wat.dataWatchers, found = removeWatcherFromList(wat.dataWatchers, w)
	case watcherTypeExist:
		wat.existWatchers, found = removeWatcherFromList(wat.existWatchers, w)
	}
	return found
}

func (c *Conn) updateWatcherZxid(w *watcher, zxid int64) {
	c.watchersLock.Lock()
	w.zxid = zxid
	c.watchersLock.Unlock()
}

func (c *Conn) queueRequest(opcode int32, req interface{}, res interface{}) <-chan response {
	rq := &request{
		xid:        c.nextXid(),
		opcode:     opcode,
		pkt:        req,
		recvStruct: res,
		recvChan:   make(chan response, 1),
	}
	c.sendChan <- rq
	return rq.recvChan
}

func (c *Conn) request(opcode int32, req interface{}, res interface{}) (int64, error) {
	r := <-c.queueRequest(opcode, req, res)
	return r.zxid, r.err
}

func (c *Conn) AddAuth(scheme string, auth []byte) error {
	_, err := c.request(opSetAuth, &setAuthRequest{Type: 0, Scheme: scheme, Auth: auth}, &setAuthResponse{})
	return err
}

func (c *Conn) Children(path string) ([]string, *Stat, error) {
	res := &getChildren2Response{}
	_, err := c.request(opGetChildren2, &getChildren2Request{Path: path, Watch: false}, res)
	return res.Children, &res.Stat, err
}

func (c *Conn) ChildrenW(path string) ([]string, *Stat, <-chan Event, error) {
	w := c.addWatcher(path, watcherTypeChild, -1)
	res := &getChildren2Response{}
	zxid, err := c.request(opGetChildren2, &getChildren2Request{Path: path, Watch: true}, res)
	if err != nil {
		c.removeWatcher(path, watcherTypeChild, w)
		return nil, nil, nil, err
	}
	c.updateWatcherZxid(w, zxid)
	return res.Children, &res.Stat, w.ch, err
}

func (c *Conn) Get(path string) ([]byte, *Stat, error) {
	res := &getDataResponse{}
	_, err := c.request(opGetData, &getDataRequest{Path: path, Watch: false}, res)
	return res.Data, &res.Stat, err
}

func (c *Conn) GetW(path string) ([]byte, *Stat, <-chan Event, error) {
	w := c.addWatcher(path, watcherTypeData, -1)
	res := &getDataResponse{}
	zxid, err := c.request(opGetData, &getDataRequest{Path: path, Watch: true}, res)
	if err != nil {
		c.removeWatcher(path, watcherTypeData, w)
		return nil, nil, nil, err
	}
	c.updateWatcherZxid(w, zxid)
	return res.Data, &res.Stat, w.ch, err
}

func (c *Conn) Set(path string, data []byte, version int32) (*Stat, error) {
	res := &setDataResponse{}
	_, err := c.request(opSetData, &SetDataRequest{path, data, version}, res)
	return &res.Stat, err
}

func (c *Conn) Create(path string, data []byte, flags int32, acl []ACL) (string, error) {
	res := &createResponse{}
	_, err := c.request(opCreate, &CreateRequest{path, data, acl, flags}, res)
	return res.Path, err
}

// Fixes a race condition if the server crashes after it creates the node. On
// reconnect the session may still be valid so the ephemeral node still exists.
// Therefore, on reconnect we need to check if a node with a GUID generated on
// create exists.
func (c *Conn) CreateProtectedEphemeralSequential(path string, data []byte, acl []ACL) (string, error) {
	var guid [16]byte
	_, err := io.ReadFull(rand.Reader, guid[:16])
	if err != nil {
		return "", err
	}
	guidStr := fmt.Sprintf("%x", guid)

	parts := strings.Split(path, "/")
	parts[len(parts)-1] = fmt.Sprintf("%s%s-%s", protectedPrefix, guidStr, parts[len(parts)-1])
	rootPath := strings.Join(parts[:len(parts)-1], "/")
	protectedPath := strings.Join(parts, "/")

	for i := 0; i < 3; i++ {
		newPath, err := c.Create(protectedPath, data, FlagEphemeral|FlagSequence, acl)
		switch err {
		case ErrSessionExpired:
			// No need to search for the node since it can't exist. Just try again.
		case ErrConnectionClosed:
			children, _, err := c.Children(rootPath)
			if err != nil {
				return "", err
			}
			for _, p := range children {
				parts := strings.Split(p, "/")
				if pth := parts[len(parts)-1]; strings.HasPrefix(pth, protectedPrefix) {
					if g := pth[len(protectedPrefix) : len(protectedPrefix)+32]; g == guidStr {
						return rootPath + "/" + p, nil
					}
				}
			}
		case nil:
			return newPath, nil
		default:
			return "", err
		}
	}
	return "", err
}

func (c *Conn) Delete(path string, version int32) error {
	_, err := c.request(opDelete, &DeleteRequest{path, version}, &deleteResponse{})
	return err
}

func (c *Conn) Exists(path string) (bool, *Stat, error) {
	res := &existsResponse{}
	_, err := c.request(opExists, &existsRequest{Path: path, Watch: false}, res)
	exists := true
	if err == ErrNoNode {
		exists = false
		err = nil
	}
	return exists, &res.Stat, err
}

func (c *Conn) ExistsW(path string) (bool, *Stat, <-chan Event, error) {
	w1 := c.addWatcher(path, watcherTypeData, -1)
	w2 := c.addWatcher(path, watcherTypeExist, -1)
	res := &existsResponse{}
	zxid, err := c.request(opExists, &existsRequest{Path: path, Watch: true}, res)
	exists := true
	if err == ErrNoNode {
		exists = false
		err = nil
	}
	if err != nil {
		c.removeWatcher(path, watcherTypeData, w1)
		c.removeWatcher(path, watcherTypeExist, w2)
		return false, nil, nil, err
	}
	var w *watcher
	if exists {
		c.removeWatcher(path, watcherTypeExist, w2)
		c.updateWatcherZxid(w1, zxid)
		w = w1
	} else {
		c.removeWatcher(path, watcherTypeExist, w1)
		c.updateWatcherZxid(w2, zxid)
		w = w2
	}
	return exists, &res.Stat, w.ch, err
}

func (c *Conn) GetACL(path string) ([]ACL, *Stat, error) {
	res := &getAclResponse{}
	_, err := c.request(opGetAcl, &getAclRequest{Path: path}, res)
	return res.Acl, &res.Stat, err
}

func (c *Conn) SetACL(path string, acl []ACL, version int32) (*Stat, error) {
	res := &setAclResponse{}
	_, err := c.request(opSetAcl, &setAclRequest{Path: path, Acl: acl, Version: version}, res)
	return &res.Stat, err
}

func (c *Conn) Sync(path string) (string, error) {
	res := &syncResponse{}
	_, err := c.request(opSync, &syncRequest{Path: path}, res)
	return res.Path, err
}

type MultiOps struct {
	Create  []CreateRequest
	Delete  []DeleteRequest
	SetData []SetDataRequest
	Check   []CheckVersionRequest
}

func (c *Conn) Multi(ops MultiOps) error {
	req := &multiRequest{
		Ops:        make([]multiRequestOp, 0, len(ops.Create)+len(ops.Delete)+len(ops.SetData)+len(ops.Check)),
		DoneHeader: multiHeader{Type: -1, Done: true, Err: -1},
	}
	for _, r := range ops.Create {
		req.Ops = append(req.Ops, multiRequestOp{multiHeader{opCreate, false, -1}, r})
	}
	for _, r := range ops.SetData {
		req.Ops = append(req.Ops, multiRequestOp{multiHeader{opSetData, false, -1}, r})
	}
	for _, r := range ops.Delete {
		req.Ops = append(req.Ops, multiRequestOp{multiHeader{opDelete, false, -1}, r})
	}
	for _, r := range ops.Check {
		req.Ops = append(req.Ops, multiRequestOp{multiHeader{opCheck, false, -1}, r})
	}
	res := &multiResponse{}
	_, err := c.request(opMulti, req, res)
	return err
}
