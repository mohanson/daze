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
	"sync/atomic"
	"time"

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

// Conf is acting as package level configuration.
var Conf = struct {
	ClientLink time.Duration
	ClientWait time.Duration
	LogClient  int
	LogServer  int
	Mtu        int
	Usr        int
}{
	ClientLink: time.Second * 4,
	ClientWait: time.Second * 8,
	LogClient:  0,
	LogServer:  0,
	Mtu:        4096,
	Usr:        256,
}

// LioConn is concurrency safe in write.
type LioConn struct {
	io.ReadWriteCloser
	mu *sync.Mutex
}

// Write implements the Conn Write method.
func (c *LioConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ReadWriteCloser.Write(p)
}

// NewLioConn returns new LioConn.
func NewLioConn(c io.ReadWriteCloser) *LioConn {
	return &LioConn{
		ReadWriteCloser: c,
		mu:              &sync.Mutex{},
	}
}

// SioConn is a combination of two pipes and it implements io.ReadWriteCloser.
type SioConn struct {
	ReaderReader *io.PipeReader
	ReaderWriter *io.PipeWriter
	WriterReader *io.PipeReader
	WriterWriter *io.PipeWriter
}

// Read implements io.Reader.
func (c *SioConn) Read(p []byte) (int, error) {
	return c.ReaderReader.Read(p)
}

// Write implements io.Writer.
func (c *SioConn) Write(p []byte) (int, error) {
	return c.WriterWriter.Write(p)
}

// Close implements io.Closer. Note that it only closes the pipe on SioConn side.
func (c *SioConn) Close() error {
	err1 := c.ReaderReader.Close()
	err2 := c.WriterWriter.Close()
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

// Esolc close the other half of the pipe.
func (c *SioConn) Esolc() error {
	err1 := c.ReaderWriter.Close()
	err2 := c.WriterReader.Close()
	if err1 != nil {
		return err1
	}
	if err2 != nil {
		return err2
	}
	return nil
}

// NewSioConn returns a new MioConn.
func NewSioConn() *SioConn {
	rr, rw := io.Pipe()
	wr, ww := io.Pipe()
	return &SioConn{
		ReaderReader: rr,
		ReaderWriter: rw,
		WriterReader: wr,
		WriterWriter: ww,
	}
}

// Server implemented the crow protocol.
type Server struct {
	Listen string
	Cipher [16]byte
	Closer io.Closer
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
		buf    = make([]byte, Conf.Mtu)
		cmd    uint8
		dst    string
		dstLen uint8
		dstNet uint8
		idx    uint16
		msgLen uint16
		srv    net.Conn
		usb    = make([]net.Conn, Conf.Usr)
	)
	for {
		_, err = io.ReadFull(cli, buf[:8])
		if err != nil {
			break
		}
		if Conf.LogServer != 0 {
			log.Printf("%s   recv data=[% x]", ctx.Cid, buf[:8])
		}
		cmd = buf[0]
		switch cmd {
		case 1:
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			buf[0] = 2
			cli.Write(buf[:8+msgLen])
		case 2:
			idx = binary.BigEndian.Uint16(buf[1:3])
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			_, err = io.ReadFull(cli, buf[8:8+msgLen])
			if err != nil {
				break
			}
			srv = usb[idx]
			if srv != nil {
				// Errors can be safely ignored. Don't ask me why, it's magic.
				srv.Write(buf[8 : 8+msgLen])
			}
		case 3:
			idx = binary.BigEndian.Uint16(buf[1:3])
			dstNet = buf[3]
			dstLen = buf[4]
			_, err = io.ReadFull(cli, buf[8:8+dstLen])
			if err != nil {
				break
			}
			dst = string(buf[8 : 8+dstLen])

			go func(idx uint16, dstNet uint8, dst string) {
				var (
					buf = make([]byte, Conf.Mtu)
					err error
					n   int
					srv net.Conn
				)
				switch dstNet {
				case 0x01:
					log.Printf("%s   dial network=tcp address=%s", ctx.Cid, dst)
					srv, err = daze.Conf.Dialer.Dial("tcp", dst)
				case 0x03:
					log.Printf("%s   dial network=udp address=%s", ctx.Cid, dst)
					srv, err = daze.Conf.Dialer.Dial("udp", dst)
				}
				buf[0] = 3
				binary.BigEndian.PutUint16(buf[1:3], idx)
				if err != nil {
					log.Println(ctx.Cid, " error", err)
					buf[3] = 1
					cli.Write(buf[:8])
					return
				}
				buf[3] = 0
				usb[idx] = srv
				cli.Write(buf[:8])
				buf[0] = 2
				for {
					n, err = srv.Read(buf[8:])
					if n != 0 {
						binary.BigEndian.PutUint16(buf[3:5], uint16(n))
						cli.Write(buf[:8+n])
					}
					if err != nil {
						break
					}
				}
				// Server close, err=EOF
				// Server crash, err=read: connection reset by peer
				// Client close  err=use of closed network connection
				if !errors.Is(err, net.ErrClosed) {
					buf[0] = 4
					cli.Write(buf[:8])
				}
				log.Printf("%s closed idx=%02x", ctx.Cid, idx)

			}(idx, dstNet, dst)
		case 4:
			idx = binary.BigEndian.Uint16(buf[1:3])
			srv = usb[idx]
			if srv != nil {
				srv.Close()
			}
		}
	}

	for _, srv := range usb {
		if srv != nil {
			srv.Close()
		}
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

// Client implemented the crow protocol.
type Client struct {
	Server string
	Cipher [16]byte
	Cli    chan io.ReadWriteCloser
	Closed uint32
	IDPool chan uint16
	Usr    []*SioConn
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	var (
		buf []byte
		err error
		idx uint16
		srv *SioConn
		cli io.ReadWriteCloser
	)
	select {
	case cli = <-c.Cli:
	case <-time.NewTimer(Conf.ClientWait).C:
		return nil, errors.New("daze: dial timeout")
	}
	idx = <-c.IDPool
	srv = NewSioConn()
	c.Usr[idx] = srv

	buf = make([]byte, 8+len(address))
	buf[0] = 3
	binary.BigEndian.PutUint16(buf[1:3], idx)
	switch network {
	case "tcp":
		buf[3] = 1
	case "udp":
		buf[3] = 3
	}
	buf[4] = uint8(len(address))
	copy(buf[8:], []byte(address))

	_, err = cli.Write(buf)
	if err != nil {
		goto Fail
	}
	_, err = io.ReadFull(srv.ReaderReader, buf[:8])
	if err != nil {
		goto Fail
	}
	if buf[3] != 0 {
		err = errors.New("daze: general server failure")
		goto Fail
	}

	go c.Proxy(ctx, srv, cli, idx)

	return srv, nil
Fail:
	srv.Close()
	srv.Esolc()
	c.IDPool <- idx
	return nil, err
}

// Proxy.
func (c *Client) Proxy(ctx *daze.Context, srv *SioConn, cli io.ReadWriteCloser, idx uint16) {
	var (
		buf = make([]byte, Conf.Mtu)
		err error
		n   int
	)
	buf[0] = 2
	binary.BigEndian.PutUint16(buf[1:3], idx)
	for {
		n, err = srv.WriterReader.Read(buf[8:])
		if n != 0 {
			binary.BigEndian.PutUint16(buf[3:5], uint16(n))
			cli.Write(buf[:8+n])
		}
		if err != nil {
			break
		}
	}
	doa.Doa(err == io.EOF || err == io.ErrClosedPipe)
	if err == io.EOF {
		buf[0] = 4
		srv.Write(buf[:8])
	}
	c.IDPool <- idx
}

// Serve creates an establish connection to crow server.
func (c *Client) Serve(ctx *daze.Context) {
	var (
		asheClient *ashe.Client
		buf        = make([]byte, Conf.Mtu)
		cli        io.ReadWriteCloser
		closedChan = make(chan int)
		cmd        uint8
		err        error
		idx        uint16
		msgLen     uint16
		srv        *SioConn
	)
	goto Tag2
Tag1:
	time.Sleep(Conf.ClientLink)
	if atomic.LoadUint32(&c.Closed) != 0 {
		return
	}
Tag2:
	cli, err = daze.Conf.Dialer.Dial("tcp", c.Server)
	if err != nil {
		log.Println(ctx.Cid, " error", err)
		goto Tag1
	}
	asheClient = &ashe.Client{Cipher: c.Cipher}
	cli, err = asheClient.WithCipher(ctx, cli)
	if err != nil {
		log.Println(ctx.Cid, " error", err)
		goto Tag1
	}
	cli = NewLioConn(cli)

	go func() {
		for {
			select {
			case c.Cli <- cli:
			case <-closedChan:
				return
			}
		}
	}()

	for {
		_, err = io.ReadFull(cli, buf[:8])
		if err != nil {
			break
		}
		if Conf.LogClient != 0 {
			log.Printf("%s   recv data=[% x]", ctx.Cid, buf[:8])
		}
		cmd = buf[0]
		switch cmd {
		case 1:
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			buf[0] = 2
			cli.Write(buf[:8+msgLen])
		case 2:
			idx = binary.BigEndian.Uint16(buf[1:3])
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			io.ReadFull(cli, buf[0:msgLen])
			srv = c.Usr[idx]
			if srv != nil {
				srv.ReaderWriter.Write(buf[:msgLen])
			}
		case 3:
			idx = binary.BigEndian.Uint16(buf[1:3])
			srv = c.Usr[idx]
			if srv != nil {
				srv.ReaderWriter.Write(buf[:8])
			}
		case 4:
			idx = binary.BigEndian.Uint16(buf[1:3])
			srv = c.Usr[idx]
			if srv != nil {
				srv.Esolc()
			}
		}
	}

	for _, srv := range c.Usr {
		if srv != nil {
			srv.Esolc()
		}
	}

	closedChan <- 0
	goto Tag1
}

// Close the static link.
func (c *Client) Close() error {
	atomic.StoreUint32(&c.Closed, 1)
	select {
	case cli := <-c.Cli:
		return cli.Close()
	default:
		return nil
	}
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher, as a sign to interface with the Server.
func NewClient(server, cipher string) *Client {
	idpool := make(chan uint16, Conf.Usr)
	for i := 1; i < Conf.Usr; i++ {
		idpool <- uint16(i)
	}
	client := &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
		Cli:    make(chan io.ReadWriteCloser),
		Closed: 0,
		IDPool: idpool,
		Usr:    make([]*SioConn, Conf.Usr),
	}
	go client.Serve(&daze.Context{Cid: "000serve"})
	return client
}
