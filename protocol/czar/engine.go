package czar

import (
	"crypto/md5"
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math"
	"net"
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

// Server implemented the czar protocol.
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

	var (
		buf      = make([]byte, Conf.MaximumTransmissionUnit)
		cmd      uint8
		dst      string
		dstLen   uint8
		dstNet   uint8
		idx      uint16
		msgLen   uint16
		priority = daze.NewPriority(2)
		srv      net.Conn
		usb      = make([]net.Conn, Conf.ConnectionPoolLimit)
	)
	for {
		_, err = io.ReadFull(cli, buf[:8])
		if err != nil {
			break
		}
		cmd = buf[0]
		switch cmd {
		case 0x01:
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			buf[0] = 0x02
			daze.Conf.Random.Read(buf[8 : 8+msgLen])
			priority.Priority(0, func() {
				cli.Write(buf[0 : 8+msgLen])
			})
		case 0x02:
			idx = binary.BigEndian.Uint16(buf[1:3])
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			_, err = io.ReadFull(cli, buf[8:8+msgLen])
			if err != nil {
				break
			}
			srv = usb[idx]
			if srv != nil {
				srv.Write(buf[8 : 8+msgLen])
			}
		case 0x03:
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
					buf = make([]byte, Conf.MaximumTransmissionUnit)
					err error
					n   int
					srv net.Conn
				)
				switch dstNet {
				case 0x01:
					log.Printf("%08x   dial network=tcp address=%s", ctx.Cid, dst)
					srv, err = daze.Conf.Dialer.Dial("tcp", dst)
				case 0x03:
					log.Printf("%08x   dial network=udp address=%s", ctx.Cid, dst)
					srv, err = daze.Conf.Dialer.Dial("udp", dst)
				}
				buf[0] = 0x03
				binary.BigEndian.PutUint16(buf[1:3], idx)
				if err != nil {
					log.Printf("%08x  error %s", ctx.Cid, err)
					buf[3] = 0x01
					priority.Priority(1, func() {
						cli.Write(buf[0:8])
					})
					return
				}
				buf[3] = 0x00
				usb[idx] = srv
				priority.Priority(1, func() {
					cli.Write(buf[0:8])
				})
				buf[0] = 0x02
				for {
					n, err = srv.Read(buf[8:])
					if n != 0 {
						binary.BigEndian.PutUint16(buf[3:5], uint16(n))
						priority.Priority(0, func() {
							cli.Write(buf[0 : 8+n])
						})
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
					priority.Priority(1, func() {
						cli.Write(buf[0:8])
					})
				}
				log.Printf("%08x closed idx=%02x", ctx.Cid, idx)
			}(idx, dstNet, dst)
		case 0x04:
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
		Cipher: md5.Sum([]byte(cipher)),
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

// Client implemented the czar protocol.
type Client struct {
	Cipher   [16]byte
	Cli      chan io.ReadWriteCloser
	Closed   uint32
	IDPool   chan uint16
	Priority *daze.Priority
	Server   string
	Usr      []*SioConn
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	var (
		buf = make([]byte, 8+len(address))
		err error
		idx uint16
		srv *SioConn
		cli io.ReadWriteCloser
	)
	select {
	case cli = <-c.Cli:
	case <-time.NewTimer(Conf.ClientDialTimeout).C:
		return nil, errors.New("daze: dial timeout")
	}
	idx = <-c.IDPool
	srv = NewSioConn()
	c.Usr[idx] = srv

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

	c.Priority.Priority(1, func() {
		_, err = cli.Write(buf)
	})
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
		buf = make([]byte, Conf.MaximumTransmissionUnit)
		err error
		n   int
	)
	buf[0] = 0x02
	binary.BigEndian.PutUint16(buf[1:3], idx)
	for {
		n, err = srv.WriterReader.Read(buf[8:])
		if n != 0 {
			binary.BigEndian.PutUint16(buf[3:5], uint16(n))
			c.Priority.Priority(0, func() {
				cli.Write(buf[0 : 8+n])
			})
		}
		if err != nil {
			break
		}
	}
	doa.Doa(err == io.EOF || err == io.ErrClosedPipe)
	if err == io.EOF {
		buf[0] = 0x04
		c.Priority.Priority(1, func() {
			cli.Write(buf[0:8])
		})
	}
	c.IDPool <- idx
}

// Serve creates an establish connection to czar server.
func (c *Client) Serve(ctx *daze.Context) {
	var (
		asheClient *ashe.Client
		buf        = make([]byte, Conf.MaximumTransmissionUnit)
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
	time.Sleep(Conf.ClientReconnectInterval)
	if atomic.LoadUint32(&c.Closed) != 0 {
		return
	}
Tag2:
	cli, err = daze.Conf.Dialer.Dial("tcp", c.Server)
	if err != nil {
		log.Printf("%08x  error %s", ctx.Cid, err)
		goto Tag1
	}
	asheClient = &ashe.Client{Cipher: c.Cipher}
	cli, err = asheClient.WithCipher(ctx, cli)
	if err != nil {
		log.Printf("%08x  error %s", ctx.Cid, err)
		goto Tag1
	}

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
		cmd = buf[0]
		switch cmd {
		case 0x01:
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			buf[0] = 0x02
			daze.Conf.Random.Read(buf[0 : 8+msgLen])
			c.Priority.Priority(0, func() {
				cli.Write(buf[0 : 8+msgLen])
			})
		case 0x02:
			idx = binary.BigEndian.Uint16(buf[1:3])
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			_, err = io.ReadFull(cli, buf[0:msgLen])
			if err != nil {
				break
			}
			srv = c.Usr[idx]
			if srv != nil {
				srv.ReaderWriter.Write(buf[:msgLen])
			}
		case 0x03:
			idx = binary.BigEndian.Uint16(buf[1:3])
			srv = c.Usr[idx]
			if srv != nil {
				srv.ReaderWriter.Write(buf[:8])
			}
		case 0x04:
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

// NewClient returns a new Client.
func NewClient(server, cipher string) *Client {
	idpool := make(chan uint16, Conf.ConnectionPoolLimit)
	for i := 1; i < Conf.ConnectionPoolLimit; i++ {
		idpool <- uint16(i)
	}
	client := &Client{
		Cipher:   md5.Sum([]byte(cipher)),
		Cli:      make(chan io.ReadWriteCloser),
		Closed:   0,
		IDPool:   idpool,
		Priority: daze.NewPriority(2),
		Server:   server,
		Usr:      make([]*SioConn, Conf.ConnectionPoolLimit),
	}
	go client.Serve(&daze.Context{Cid: math.MaxUint32})
	return client
}
