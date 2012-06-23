package zk

// TODO: ping server
// TODO: everything

import (
	"encoding/binary"
	"io"
	"log"
	"net"
	"time"
)

type state int

type Conn struct {
	conn         net.Conn
	state        state
	eventChan    <-chan Event
	pingInterval time.Duration

	lastZxid  int32
	sessionId int64
	timeout   int32
	passwd    []byte
}

type Event struct {
}

func Dial(servers []string, recvTimeout time.Duration) (*Conn, <-chan Event, error) {
	zkConn, err := net.Dial("tcp", servers[0])
	if err != nil {
		return nil, nil, err
	}
	ec := make(<-chan Event, 5)
	conn := Conn{
		conn:         zkConn,
		state:        stateUnassociated,
		eventChan:    ec,
		pingInterval: 10 * time.Second,
	}
	go conn.loop()
	return &conn, ec, nil
}

func (c *Conn) loop() {
	buf := make([]byte, 1024)

	req := connectRequest{
		ProtocolVersion: protocolVersion,
		LastZxidSeen:    0,
		TimeOut:         30000,
		SessionId:       0,
		Passwd:          emptyPassword,
	}
	n, err := encodePacket(buf, &req)
	if err != nil {
		// TODO
		panic(err)
	}

	_, err = c.conn.Write(buf[:n])
	if err != nil {
		// TODO: Send an event and change state
		panic(err)
	}

	for {
		_, err = io.ReadFull(c.conn, buf[:4])
		if err != nil {
			// TODO
			panic(err)
		}

		blen := int(binary.BigEndian.Uint32(buf[:4]))
		if cap(buf) < blen {
			buf = make([]byte, blen)
		}

		_, err = io.ReadFull(c.conn, buf[:blen])
		if err != nil {
			// TODO
			panic(err)
		}

		if c.state == stateUnassociated {
			r := connectResponse{}
			decodePacket(buf[:blen], r)
			c.timeout = r.TimeOut
			c.sessionId = r.SessionId
			c.passwd = r.Passwd
			c.state = stateSyncConnected
		} else {
			// TODO
		}

		log.Printf("%+v\n", buf[:blen])
	}
}
