package crow

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"sync"

	"github.com/godump/doa"
	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
)

// The crow protocol is a proxy protocol built on TCP multiplexing technology. It eliminates some common characteristics
// of proxy software, such as frequent connection establishment and disconnection when browsing websites. It makes it
// more difficult to be detected by firewalls.
//
// When the client is initialized, it needs to establish and maintain a connection with the server. After that, the
// client and server communicate through the following commands.
//
// The server and client can request each other to send random data of a specified size and simply discarded.
// The purpose of this command is to shape traffic to avoid being identified.
//
// +-----+-----+-----+-----+-----+-----+-----+-----+
// |  1  |    Idx    |    Len    |       Rsv       |
// +-----+-----+-----+-----+-----+-----+-----+-----+
//
// Both server and client can push data to each other. The ID in command can be obtained in next command.
//
// +-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+
// |  2  |    Idx    |    Len    |       Rsv       |                      Msg                      |
// +-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+
//
// Client wishes to establish a connection. The client needs to transmit two network and destination address. The server
// will typically evaluate the request based on network and destination addresses, and return one reply messages, as
// appropriate for the request type.
//
// +-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+
// |  3  |    Idx    | Net | Len |       Rsv       |                      Dst                      |
// +-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+
//
// +-----+-----+-----+-----+-----+-----+-----+-----+
// |  3  |    Idx    | Rep |          Rsv          |
// +-----+-----+-----+-----+-----+-----+-----+-----+
//
// Net: 0x01 TCP
//      0x03 UDP
// Rep: 0x00 succeeded
//      0x01 general failure
//
// Close the specified connection.
//
// +-----+-----+-----+-----+-----+-----+-----+-----+
// |  4  |    Idx    |             Rsv             |
// +-----+-----+-----+-----+-----+-----+-----+-----+

// LioConn is concurrency safe in write.
type LioConn struct {
	io.ReadWriteCloser
	l *sync.Mutex
}

// Write implements the Conn Write method.
func (c *LioConn) Write(p []byte) (int, error) {
	c.l.Lock()
	defer c.l.Unlock()
	return c.ReadWriteCloser.Write(p)
}

// NewLioConn returns new LioConn.
func NewLioConn(c io.ReadWriteCloser) *LioConn {
	return &LioConn{
		ReadWriteCloser: c,
		l:               &sync.Mutex{},
	}
}

// SioConn wraps conn and provides some property values.
type SioConn struct {
	io.ReadWriteCloser
	Closed int
	Idx    uint16
}

// NewSioConn returns a new SioConn. It is the caller's responsibility to set additional properties.
func NewSioConn(c io.ReadWriteCloser) *SioConn {
	return &SioConn{
		ReadWriteCloser: c,
	}
}

// Server implemented the crow protocol.
type Server struct {
	Listen string
	Cipher [16]byte
	Closer io.Closer
}

// ServeSio.
func (s *Server) ServeSio(ctx *daze.Context, sio *SioConn, cli io.ReadWriteCloser) {
	buf := make([]byte, 2048)
	for {
		n, err := sio.Read(buf[8:])
		if n != 0 {
			buf[0] = 2
			binary.BigEndian.PutUint16(buf[1:3], sio.Idx)
			binary.BigEndian.PutUint16(buf[3:5], uint16(n))
			cli.Write(buf[:8+n])
		}
		if err != nil {
			break
		}
	}
	if sio.Closed == 0 {
		buf[0] = 4
		binary.BigEndian.PutUint16(buf[1:3], sio.Idx)
		cli.Write(buf[:8])
		sio.Closed = 1
	}
	log.Println(ctx.Cid, "closed")
}

// Serve. Parameter raw will be closed automatically when the function exits.
func (s *Server) Serve(ctx *daze.Context, raw io.ReadWriteCloser) error {
	var (
		asheServer *ashe.Server
		cli        io.ReadWriteCloser
		err        error
	)
	asheServer = &ashe.Server{Cipher: s.Cipher}
	cli, err = asheServer.ServeCipher(ctx, raw)
	if err != nil {
		return err
	}
	cli = NewLioConn(cli)
	var (
		buf          = make([]byte, 2048)
		dst          string
		harbor       = map[uint16]*SioConn{}
		headerCmd    uint8
		headerDstLen uint8
		headerDstNet uint8
		headerIdx    uint16
		headerMsgLen uint16
		ok           bool
		srv          io.ReadWriteCloser
		sio          *SioConn
	)
	for {
		_, err = io.ReadFull(cli, buf[:8])
		if err != nil {
			break
		}
		log.Printf("%s   recv data=0x%x", ctx.Cid, buf[:8])
		headerCmd = buf[0]
		switch headerCmd {
		case 1:
			headerMsgLen = binary.BigEndian.Uint16(buf[3:5])
			doa.Doa(headerMsgLen <= 2040)
			buf[0] = 2
			cli.Write(buf[:8+headerMsgLen])
		case 2:
			headerIdx = binary.BigEndian.Uint16(buf[1:3])
			headerMsgLen = binary.BigEndian.Uint16(buf[3:5])
			doa.Doa(headerMsgLen <= 2040)
			_, err = io.ReadFull(cli, buf[8:8+headerMsgLen])
			if err != nil {
				break
			}
			sio, ok = harbor[headerIdx]
			if ok && sio.Closed == 0 {
				sio.Write(buf[8 : 8+headerMsgLen])
			}
		case 3:
			headerIdx = binary.BigEndian.Uint16(buf[1:3])
			headerDstNet = buf[3]
			headerDstLen = buf[4]
			_, err = io.ReadFull(cli, buf[8:8+headerDstLen])
			if err != nil {
				break
			}
			dst = string(buf[8 : 8+headerDstLen])
			switch headerDstNet {
			case 0x01:
				log.Printf("%s   dial network=tcp address=%s", ctx.Cid, dst)
				srv, err = daze.Conf.Dialer.Dial("tcp", dst)
				srv = ashe.NewTCPConn(srv)
				sio = NewSioConn(srv)
			case 0x03:
				log.Printf("%s   dial network=udp address=%s", ctx.Cid, dst)
				srv, err = daze.Conf.Dialer.Dial("udp", dst)
				srv = ashe.NewUDPConn(srv)
				sio = NewSioConn(srv)
			}
			sio.Idx = headerIdx
			harbor[headerIdx] = sio
			buf[0] = 3
			if err != nil {
				buf[3] = 1
			} else {
				buf[3] = 0
			}
			cli.Write(buf[:8])
			go s.ServeSio(ctx, sio, cli)
		case 4:
			headerIdx = binary.BigEndian.Uint16(buf[1:3])
			sio, ok = harbor[headerIdx]
			if ok && sio.Closed == 0 {
				sio.Closed = 1
				sio.Close()
			}
		}
	}

	for _, e := range harbor {
		e.Closed = 1
		e.Close()
	}

	return nil
}

// Close listener.
func (s *Server) Close() error {
	if s.Closer != nil {
		return s.Closer.Close()
	}
	return nil
}

// Run.
func (s *Server) Run() error {
	ln, err := net.Listen("tcp", s.Listen)
	if err != nil {
		return err
	}
	s.Closer = ln
	log.Println("listen and serve on", s.Listen)

	go func() {
		idx := uint32(math.MaxUint32)
		for {
			cli, err := ln.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Println(err)
				}
				break
			}
			idx += 1
			ctx := &daze.Context{Cid: daze.Hu32(idx)}
			log.Printf("%s accept remote=%s", ctx.Cid, cli.RemoteAddr())
			go func(cli net.Conn) {
				defer cli.Close()
				if err := s.Serve(ctx, cli); err != nil {
					log.Println(ctx.Cid, " error", err)
				}
				log.Println(ctx.Cid, "closed")
			}(cli)
		}
	}()

	return nil
}

// NewServer returns a new Server. A secret data needs to be passed in Cipher, as a sign to interface with the Client.
func NewServer(listen string, cipher string) *Server {
	return &Server{
		Listen: listen,
		Cipher: md5.Sum([]byte(cipher)),
	}
}

// MioConn.
type MioConn struct {
	Closed int
	Prr    *io.PipeReader
	Prw    *io.PipeWriter
	Pwr    *io.PipeReader
	Pww    *io.PipeWriter
}

func (c *MioConn) Read(p []byte) (int, error) {
	return c.Prr.Read(p)
}

func (c *MioConn) Write(p []byte) (int, error) {
	return c.Pww.Write(p)
}

func (c *MioConn) Close() error {
	err1 := c.Prr.Close()
	err2 := c.Pww.Close()
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

func NewMioConn() *MioConn {
	rr, rw := io.Pipe()
	wr, ww := io.Pipe()
	return &MioConn{
		Prr: rr,
		Prw: rw,
		Pwr: wr,
		Pww: ww,
	}
}

// Client implemented the crow protocol.
type Client struct {
	Server string
	Cipher [16]byte
	Harbor map[uint16]*MioConn
	IDPool chan uint16
	Lio    io.ReadWriteCloser
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	var (
		buf = make([]byte, 8+len(address))
		idx uint16
		mio *MioConn
	)
	buf[0] = 3
	idx = <-c.IDPool
	binary.BigEndian.PutUint16(buf[1:3], idx)
	switch network {
	case "tcp":
		buf[3] = 1
	case "udp":
		buf[3] = 3
	}
	buf[4] = uint8(len(address))
	copy(buf[8:], []byte(address))
	c.Lio.Write(buf)

	mio = NewMioConn()
	c.Harbor[idx] = mio

	go func() {
		buf := make([]byte, 2048)
		buf[0] = 2
		binary.BigEndian.PutUint16(buf[1:3], idx)
		for {
			n, err := mio.Pwr.Read(buf[8:])
			if n != 0 {
				binary.BigEndian.PutUint16(buf[3:5], uint16(n))
				c.Lio.Write(buf[:8+n])
			}
			if err != nil {
				break
			}
		}
		if mio.Closed == 0 {
			buf := make([]byte, 8)
			buf[0] = 4
			binary.BigEndian.PutUint16(buf[1:3], idx)
			c.Lio.Write(buf)
			mio.Closed = 1
		}
		c.IDPool <- idx
	}()

	io.ReadFull(mio.Prr, buf[:8])
	doa.Doa(buf[0] == 3)
	if buf[3] != 0 {
		mio.Closed = 1
		mio.Prr.Close()
		mio.Pww.Close()
		return nil, errors.New("daze: general server failure")
	}
	return mio, nil
}

// Run creates an establish connection to crow server.
func (c *Client) Run() error {
	if c.Lio != nil {
		return nil
	}
	var (
		asheClient *ashe.Client
		srv        io.ReadWriteCloser
		err        error
	)
	srv, err = daze.Conf.Dialer.Dial("tcp", c.Server)
	if err != nil {
		return err
	}
	asheClient = &ashe.Client{Cipher: c.Cipher}
	srv, err = asheClient.WithCipher(&daze.Context{Cid: "cccccccc"}, srv)
	if err != nil {
		return err
	}
	srv = NewLioConn(srv)
	c.Lio = srv

	go func() {
		var (
			buf          = make([]byte, 2048)
			err          error
			mio          *MioConn
			headerCmd    uint8
			headerIdx    uint16
			headerMsgLen uint16
			ok           bool
		)
		for {
			_, err = io.ReadFull(srv, buf[:8])
			if err != nil {
				break
			}
			log.Printf("%s   recv data=0x%x", "cccccccc", buf[:8])
			headerCmd = buf[0]
			switch headerCmd {
			case 1:
				headerMsgLen = binary.BigEndian.Uint16(buf[3:5])
				doa.Doa(headerMsgLen <= 2040)
				buf[0] = 2
				srv.Write(buf[:8+headerMsgLen])
			case 2:
				headerIdx = binary.BigEndian.Uint16(buf[1:3])
				headerMsgLen = binary.BigEndian.Uint16(buf[3:5])
				doa.Doa(int(headerMsgLen) <= 2040)
				_, err = io.ReadFull(srv, buf[:headerMsgLen])
				if err != nil {
					break
				}
				mio, ok = c.Harbor[headerIdx]
				if ok {
					mio.Prw.Write(buf[0:headerMsgLen])
				}
			case 3:
				headerIdx = binary.BigEndian.Uint16(buf[1:3])
				mio, ok = c.Harbor[headerIdx]
				if ok {
					mio.Prw.Write(buf[:8])
				}
			case 4:
				headerIdx = binary.BigEndian.Uint16(buf[1:3])
				mio, ok = c.Harbor[headerIdx]
				if ok {
					mio.Closed = 1
					mio.Prw.Close()
					mio.Pwr.Close()
				}
			}
		}
	}()

	return nil
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher, as a sign to interface with the Server.
func NewClient(server, cipher string) *Client {
	idpool := make(chan uint16, 256)
	for i := uint16(1); i < 256; i++ {
		idpool <- i
	}
	return &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
		Harbor: map[uint16]*MioConn{},
		IDPool: idpool,
		Lio:    nil,
	}
}
