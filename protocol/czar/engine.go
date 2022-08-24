package czar

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godump/doa"
	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
)

// The czar protocol is a proxy protocol built on TCP multiplexing technology. By establishing multiple TCP connections
// in one TCP channel, czar protocol effectively reduces the consumption of establishing connections between the client
// and the server:
//
// Client port: a.com ------------┐                   ┌------------ Server port: a.com
// Client port: b.com ----------┐ |                   | ┌---------- Server port: b.com
// Client port: c.com ----------+-+-- czar protocol --+-+---------- Server port: c.com
// Client port: d.com ----------┘ |                   | └---------- Server port: d.com
// Client port: e.com ------------┘                   └------------ Server port: e.com
//
// When the client is initialized, it needs to establish and maintain a connection with the server. After that, the
// client and server communicate through the following commands.
//
// The server and client can request each other to send random data of a specified size. The purpose of this command is
// to shape traffic to avoid being identified.
//
// +-----+-----+-----+-----+-----+-----+-----+-----+
// |  1  |    Idx    |    Len    |       Rsv       |
// +-----+-----+-----+-----+-----+-----+-----+-----+
//
// Both server and client can push data to each other.
//
// +-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+
// |  2  |    Idx    |    Len    |       Rsv       |                      Msg                      |
// +-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+
//
// Client requests to establish a connection and marks the connection with an idx. The server will typically evaluate
// the request based on network and destination addresses, and return one reply messages.
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

// Conf is acting as package level configuration.
var Conf = struct {
	ClientDialTimeout       time.Duration
	ClientReconnectInterval time.Duration
	ConnectionPoolLimit     int
	MaximumTransmissionUnit int
}{
	ClientDialTimeout:       time.Second * 8,
	ClientReconnectInterval: time.Second * 4,
	ConnectionPoolLimit:     256,
	MaximumTransmissionUnit: 4096,
}

// ServerConn represents a single conn on czar server.
type ServerConn struct {
	cli io.ReadWriteCloser
	ctx *daze.Context
	pri *daze.Priority
	srv []io.ReadWriteCloser
	wg1 sync.WaitGroup
}

// Serve incoming connections.
func (s *ServerConn) Serve() {
	var (
		buf    = make([]byte, Conf.MaximumTransmissionUnit)
		cmd    uint8
		dst    string
		dstLen uint8
		dstNet uint8
		err    error
		idx    uint16
		msgLen uint16
		rwc    io.ReadWriteCloser
	)
	for {
		_, err = io.ReadFull(s.cli, buf[:8])
		if err != nil {
			log.Printf("%08x  error %s", s.ctx.Cid, err)
			break
		}
		cmd = buf[0]
		idx = binary.BigEndian.Uint16(buf[1:3])
		switch cmd {
		case 0x01:
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			buf[0] = 0x02
			_, err = daze.Random.Read(buf[8 : 8+msgLen])
			if err != nil {
				break
			}
			err = s.WritePri0(buf[0 : 8+msgLen])
		case 0x02:
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			_, err = io.ReadFull(s.cli, buf[8:8+msgLen])
			if err != nil {
				break
			}
			rwc = s.srv[idx]
			_, err = rwc.Write(buf[8 : 8+msgLen])
		case 0x03:
			dstNet = buf[3]
			dstLen = buf[4]
			_, err = io.ReadFull(s.cli, buf[8:8+dstLen])
			if err != nil {
				break
			}
			dst = string(buf[8 : 8+dstLen])
			s.wg1.Add(1)
			go s.serve(idx, dstNet, dst)
		case 0x04:
			rwc = s.srv[idx]
			err = rwc.Close()
		}
		if err != nil {
			log.Printf("%08x  error idx=%02x %s", s.ctx.Cid, idx, err)
		}
	}

	s.wg1.Wait()
	for _, rwc = range s.srv {
		rwc.Close()
	}
}

func (s *ServerConn) serve(idx uint16, dstNet uint8, dst string) {
	var (
		buf = make([]byte, Conf.MaximumTransmissionUnit)
		err error
		n   int
		rwc net.Conn
	)
	buf[0] = 0x03
	binary.BigEndian.PutUint16(buf[1:3], idx)
	switch dstNet {
	case 0x01:
		log.Printf("%08x   dial idx=%02x network=tcp address=%s", s.ctx.Cid, idx, dst)
		rwc, err = daze.Dial("tcp", dst)
	case 0x03:
		log.Printf("%08x   dial idx=%02x network=udp address=%s", s.ctx.Cid, idx, dst)
		rwc, err = daze.Dial("udp", dst)
	}
	if err != nil {
		s.wg1.Done()
		log.Printf("%08x  error idx=%02x %s", s.ctx.Cid, idx, err)
		buf[3] = 0x01
		s.WritePri1(buf[0:8])
		return
	}
	buf[3] = 0x00
	s.srv[idx] = rwc
	s.wg1.Done()
	s.WritePri1(buf[0:8])
	buf[0] = 0x02
	for {
		n, err = rwc.Read(buf[8:])
		if n != 0 {
			binary.BigEndian.PutUint16(buf[3:5], uint16(n))
			s.WritePri0(buf[0 : 8+n])
		}
		if err != nil {
			break
		}
	}
	// Server close, err=EOF
	// Server crash, err=read: connection reset by peer
	// Client close  err=use of closed network connection
	if !errors.Is(err, net.ErrClosed) {
		buf[0] = 0x04
		s.WritePri1(buf[0:8])
	}
	log.Printf("%08x closed idx=%02x", s.ctx.Cid, idx)
}

func (s *ServerConn) WritePri0(p []byte) error {
	return s.pri.Priority(0, func() error {
		return doa.Err(s.cli.Write(p))
	})
}

func (s *ServerConn) WritePri1(p []byte) error {
	return s.pri.Priority(0, func() error {
		return doa.Err(s.cli.Write(p))
	})
}

// NewServerConn returns a new ServerConn.
func NewServerConn(ctx *daze.Context, cli io.ReadWriteCloser) *ServerConn {
	srv := make([]io.ReadWriteCloser, Conf.ConnectionPoolLimit)
	for i := 0; i < len(srv); i++ {
		rwc := NewClientConn()
		rwc.Close()
		rwc.Esolc()
		srv[i] = rwc
	}
	return &ServerConn{
		cli: cli,
		ctx: ctx,
		pri: daze.NewPriority(2),
		srv: srv,
		wg1: sync.WaitGroup{},
	}
}

// Server implemented the czar protocol.
type Server struct {
	Listen string
	Cipher []byte
	Closer io.Closer
}

// Serve incoming connections. Parameter cli will be closed automatically when the function exits.
func (s *Server) Serve(ctx *daze.Context, cli io.ReadWriteCloser) error {
	var (
		asheServer *ashe.Server
		err        error
		serverConn *ServerConn
	)
	asheServer = &ashe.Server{Cipher: s.Cipher}
	cli, err = asheServer.ServeCipher(ctx, cli)
	if err != nil {
		return err
	}
	serverConn = NewServerConn(ctx, cli)
	serverConn.Serve()
	return nil
}

// Close listener.
func (s *Server) Close() error {
	if s.Closer != nil {
		return s.Closer.Close()
	}
	return nil
}

// Run it.
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
			idx++
			ctx := &daze.Context{Cid: idx}
			log.Printf("%08x accept remote=%s", ctx.Cid, cli.RemoteAddr())
			go func(cli net.Conn) {
				defer cli.Close()
				if err := s.Serve(ctx, cli); err != nil {
					log.Printf("%08x  error %s", ctx.Cid, err)
				}
				log.Printf("%08x closed", ctx.Cid)
			}(cli)
		}
	}()

	return nil
}

// NewServer returns a new Server.
func NewServer(listen string, cipher string) *Server {
	return &Server{
		Listen: listen,
		Cipher: daze.Salt(cipher),
	}
}

// ClientConn is a combination of two pipes and it implements io.ReadWriteCloser.
type ClientConn struct {
	rr *io.PipeReader
	rw *io.PipeWriter
	wr *io.PipeReader
	ww *io.PipeWriter
}

// Read implements io.Reader.
func (c *ClientConn) Read(p []byte) (int, error) {
	return c.rr.Read(p)
}

// Write implements io.Writer.
func (c *ClientConn) Write(p []byte) (int, error) {
	return c.ww.Write(p)
}

// Close implements io.Closer. Note that it only closes the pipe on ClientConn side.
func (c *ClientConn) Close() error {
	if err := c.rr.Close(); err != nil {
		return err
	}
	if err := c.ww.Close(); err != nil {
		return err
	}
	return nil
}

// Esolc close the other half of the pipe.
func (c *ClientConn) Esolc() error {
	if err := c.rw.Close(); err != nil {
		return err
	}
	if err := c.wr.Close(); err != nil {
		return err
	}
	return nil
}

// NewClientConn returns a new MioConn.
func NewClientConn() *ClientConn {
	rr, rw := io.Pipe()
	wr, ww := io.Pipe()
	return &ClientConn{
		rr: rr,
		rw: rw,
		wr: wr,
		ww: ww,
	}
}

// Client implemented the czar protocol.
type Client struct {
	Cipher []byte
	Cli    []*ClientConn
	Closed uint32
	IDPool chan uint16
	Pri    *daze.Priority
	Server string
	Srv    chan io.ReadWriteCloser
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	var (
		buf = make([]byte, 8+len(address))
		err error
		idx uint16
		cli *ClientConn
		srv io.ReadWriteCloser
	)
	select {
	case srv = <-c.Srv:
	case <-time.NewTimer(Conf.ClientDialTimeout).C:
		return nil, errors.New("daze: dial timeout")
	}
	idx = <-c.IDPool
	cli = NewClientConn()
	c.Cli[idx] = cli

	buf[0] = 0x03
	binary.BigEndian.PutUint16(buf[1:3], idx)
	switch network {
	case "tcp":
		buf[3] = 0x01
	case "udp":
		buf[3] = 0x03
	}
	buf[4] = uint8(len(address))
	copy(buf[8:], []byte(address))

	err = c.Pri.Priority(1, func() error {
		return doa.Err(srv.Write(buf))
	})
	if err != nil {
		goto Fail
	}
	_, err = io.ReadFull(cli.rr, buf[:8])
	if err != nil {
		goto Fail
	}
	if buf[3] != 0 {
		err = errors.New("daze: general server failure")
		goto Fail
	}

	go c.Proxy(ctx, cli, srv, idx)

	return cli, nil
Fail:
	cli.Close()
	cli.Esolc()
	c.IDPool <- idx
	return nil, err
}

// Proxy conn.
func (c *Client) Proxy(ctx *daze.Context, cli *ClientConn, srv io.ReadWriteCloser, idx uint16) {
	var (
		buf = make([]byte, Conf.MaximumTransmissionUnit)
		err error
		n   int
	)
	buf[0] = 0x02
	binary.BigEndian.PutUint16(buf[1:3], idx)
	for {
		n, err = cli.wr.Read(buf[8:])
		if n != 0 {
			binary.BigEndian.PutUint16(buf[3:5], uint16(n))
			c.Pri.Priority(0, func() error {
				return doa.Err(srv.Write(buf[0 : 8+n]))
			})
		}
		if err != nil {
			break
		}
	}
	doa.Doa(err == io.EOF || err == io.ErrClosedPipe)
	if err == io.EOF {
		buf[0] = 0x04
		c.Pri.Priority(1, func() error {
			return doa.Err(srv.Write(buf[0:8]))
		})
	}
	c.IDPool <- idx
}

// Serve creates an establish connection to czar server.
func (c *Client) Serve(ctx *daze.Context) {
	var (
		asheClient *ashe.Client
		buf        = make([]byte, Conf.MaximumTransmissionUnit)
		cli        *ClientConn
		closedChan = make(chan int)
		cmd        uint8
		err        error
		idx        uint16
		msgLen     uint16
		srv        io.ReadWriteCloser
	)
	goto Tag2
Tag1:
	time.Sleep(Conf.ClientReconnectInterval)
	if atomic.LoadUint32(&c.Closed) != 0 {
		return
	}
Tag2:
	srv, err = daze.Dial("tcp", c.Server)
	if err != nil {
		log.Printf("%08x  error %s", ctx.Cid, err)
		goto Tag1
	}
	asheClient = &ashe.Client{Cipher: c.Cipher}
	srv, err = asheClient.WithCipher(ctx, srv)
	if err != nil {
		log.Printf("%08x  error %s", ctx.Cid, err)
		goto Tag1
	}

	go func() {
		for {
			select {
			case c.Srv <- srv:
			case <-closedChan:
				return
			}
		}
	}()

	for {
		_, err = io.ReadFull(srv, buf[:8])
		if err != nil {
			log.Printf("%08x  error %s", ctx.Cid, err)
			break
		}
		cmd = buf[0]
		switch cmd {
		case 0x01:
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			buf[0] = 0x02
			daze.Random.Read(buf[0 : 8+msgLen])
			err = c.Pri.Priority(0, func() error {
				return doa.Err(srv.Write(buf[0 : 8+msgLen]))
			})
		case 0x02:
			idx = binary.BigEndian.Uint16(buf[1:3])
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			_, err = io.ReadFull(srv, buf[0:msgLen])
			if err != nil {
				break
			}
			cli = c.Cli[idx]
			_, err = cli.rw.Write(buf[:msgLen])
		case 0x03:
			idx = binary.BigEndian.Uint16(buf[1:3])
			cli = c.Cli[idx]
			_, err = cli.rw.Write(buf[:8])
		case 0x04:
			idx = binary.BigEndian.Uint16(buf[1:3])
			cli = c.Cli[idx]
			err = cli.Esolc()
		}
		if err != nil {
			log.Printf("%08x  error %s", ctx.Cid, err)
		}
	}

	for _, cli = range c.Cli {
		cli.Esolc()
	}

	closedChan <- 0
	goto Tag1
}

// Close the static link.
func (c *Client) Close() error {
	atomic.StoreUint32(&c.Closed, 1)
	select {
	case srv := <-c.Srv:
		return srv.Close()
	default:
		return nil
	}
}

// NewClient returns a new Client.
func NewClient(server, cipher string) *Client {
	idpool := make(chan uint16, Conf.ConnectionPoolLimit)
	for i := 1; i < Conf.ConnectionPoolLimit; i++ {
		idpool <- uint16(i)
	}
	cli := make([]*ClientConn, Conf.ConnectionPoolLimit)
	for i := 0; i < len(cli); i++ {
		rwc := NewClientConn()
		rwc.Close()
		rwc.Esolc()
		cli[i] = rwc
	}
	client := &Client{
		Cipher: daze.Salt(cipher),
		Cli:    cli,
		Closed: 0,
		IDPool: idpool,
		Pri:    daze.NewPriority(2),
		Server: server,
		Srv:    make(chan io.ReadWriteCloser),
	}
	go client.Serve(&daze.Context{Cid: math.MaxUint32})
	return client
}
