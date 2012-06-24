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
	servers        []string
	serverIndex    int
	conn           net.Conn
	state          state
	eventChan      <-chan Event
	pingInterval   time.Duration
	recvTimeout    time.Duration
	connectTimeout time.Duration

	lastZxid  int64
	sessionId int64
	timeout   int32
	passwd    []byte
}

type Event struct {
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

		buf := make([]byte, 1024)

		n, err := encodePacket(buf, &connectRequest{
			ProtocolVersion: protocolVersion,
			LastZxidSeen:    c.lastZxid,
			TimeOut:         30000,
			SessionId:       c.sessionId,
			Passwd:          emptyPassword,
		})
		if err != nil {
			log.Println(err)
			continue
		}

		_, err = c.conn.Write(buf[:n])
		if err != nil {
			log.Println(err)
			continue
		}

		for {
			_, err = io.ReadFull(c.conn, buf[:4])
			if err != nil {
				log.Println(err)
				break
			}

			blen := int(binary.BigEndian.Uint32(buf[:4]))
			if cap(buf) < blen {
				buf = make([]byte, blen)
			}

			_, err = io.ReadFull(c.conn, buf[:blen])
			if err != nil {
				log.Println(err)
				break
			}

			if c.state == stateConnected {
				r := connectResponse{}
				decodePacket(buf[:blen], r)
				// TODO: Check for a dead session
				c.timeout = r.TimeOut
				c.sessionId = r.SessionId
				c.passwd = r.Passwd
				c.state = stateHasSession
			} else {
				// TODO
			}

			log.Printf("%+v\n", buf[:blen])
		}
	}
}
