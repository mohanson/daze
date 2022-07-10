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

var Conf = struct {
	ClientLinkRetry   time.Duration
	ClientLinkTimeout time.Duration
	LogClient         int
	LogServer         int
}{
	ClientLinkRetry:   time.Second * 4,
	ClientLinkTimeout: time.Second * 8,
	LogClient:         0,
	LogServer:         0,
}

// LioConn is concurrency safe in write.
type LioConn struct {
	io.ReadWriteCloser
	Lock *sync.Mutex
}

// Write implements the Conn Write method.
func (c *LioConn) Write(p []byte) (int, error) {
	c.Lock.Lock()
	defer c.Lock.Unlock()
	return c.ReadWriteCloser.Write(p)
}

// NewLioConn returns new LioConn.
func NewLioConn(c io.ReadWriteCloser) *LioConn {
	return &LioConn{
		ReadWriteCloser: c,
		Lock:            &sync.Mutex{},
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

// CloseOther close the other half of the pipe.
func (c *SioConn) CloseOther() error {
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

// ServeSio.
func (s *Server) ServeSio(ctx *daze.Context, sio *SioConn, cli io.ReadWriteCloser, idx uint16) {
	var (
		buf = make([]byte, 2048)
		err error
		n   int
	)
	buf[0] = 2
	binary.BigEndian.PutUint16(buf[1:3], idx)
	for {
		n, err = sio.WriterReader.Read(buf[8:])
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
		cli.Write(buf[:8])
	}
	log.Printf("%s closed idx=%02x", ctx.Cid, idx)
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
		buf    = make([]byte, 2048)
		cmd    uint8
		dst    string
		dstLen uint8
		dstNet uint8
		harbor = map[uint16]*SioConn{}
		idx    uint16
		msgLen uint16
		ok     bool
		srv    io.ReadWriteCloser
		sio    *SioConn
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
			doa.Doa(msgLen <= 2040)
			buf[0] = 2
			cli.Write(buf[:8+msgLen])
		case 2:
			idx = binary.BigEndian.Uint16(buf[1:3])
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			doa.Doa(msgLen <= 2040)
			_, err = io.ReadFull(cli, buf[8:8+msgLen])
			if err != nil {
				break
			}
			sio, ok = harbor[idx]
			doa.Doa(ok)
			if ok {
				// Errors can be safely ignored. Don't ask me why, it's magic.
				sio.ReaderWriter.Write(buf[8 : 8+msgLen])
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
			switch dstNet {
			case 0x01:
				log.Printf("%s   dial network=tcp address=%s", ctx.Cid, dst)
				srv, err = daze.Conf.Dialer.Dial("tcp", dst)
				srv = ashe.NewTCPConn(srv)
			case 0x03:
				log.Printf("%s   dial network=udp address=%s", ctx.Cid, dst)
				srv, err = daze.Conf.Dialer.Dial("udp", dst)
				srv = ashe.NewUDPConn(srv)
			}
			if err == nil {
				sio = NewSioConn()
				harbor[idx] = sio
				go daze.Link(srv, sio)
				go s.ServeSio(ctx, sio, cli, idx)
			} else {
				log.Println(ctx.Cid, " error", err)
			}
			buf[0] = 3
			if err != nil {
				buf[3] = 1
			} else {
				buf[3] = 0
			}
			cli.Write(buf[:8])
		case 4:
			idx = binary.BigEndian.Uint16(buf[1:3])
			sio, ok = harbor[idx]
			doa.Doa(ok)
			if ok {
				sio.CloseOther()
			}
		}
	}

	for _, sio := range harbor {
		sio.CloseOther()
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
	Closed chan int
	Harbor map[uint16]*SioConn
	IDPool chan uint16
	Srv    chan io.ReadWriteCloser
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	var (
		buf = make([]byte, 8+len(address))
		err error
		idx = <-c.IDPool
		sio = NewSioConn()
		srv io.ReadWriteCloser
	)
	select {
	case srv = <-c.Srv:
	case <-time.NewTimer(Conf.ClientLinkTimeout).C:
		c.IDPool <- idx
		return nil, errors.New("daze: dial timeout")
	}
	c.Harbor[idx] = sio
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
	_, err = srv.Write(buf)
	if err != nil {
		c.IDPool <- idx
		return nil, err
	}
	_, err = io.ReadFull(sio.ReaderReader, buf[:8])
	if err != nil || buf[3] != 0 {
		c.IDPool <- idx
		return nil, errors.New("daze: general server failure")
	}
	go c.Proxy(ctx, sio, srv, idx)
	return sio, nil
}

// Proxy.
func (c *Client) Proxy(ctx *daze.Context, sio *SioConn, srv io.ReadWriteCloser, idx uint16) {
	var (
		buf = make([]byte, 2048)
		err error
		n   int
	)
	buf[0] = 2
	binary.BigEndian.PutUint16(buf[1:3], idx)
	for {
		n, err = sio.WriterReader.Read(buf[8:])
		if n != 0 {
			binary.BigEndian.PutUint16(buf[3:5], uint16(n))
			srv.Write(buf[:8+n])
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
func (c *Client) Serve() {
	var (
		asheClient *ashe.Client
		closedChan = make(chan int)
		err        error
		srv        io.ReadWriteCloser
	)
	goto Tag2
Tag1:
	time.Sleep(Conf.ClientLinkRetry)
Tag2:
	select {
	case <-c.Closed:
		return
	default:
	}
	srv, err = daze.Conf.Dialer.Dial("tcp", c.Server)
	if err != nil {
		log.Println(err)
		goto Tag1
	}
	asheClient = &ashe.Client{Cipher: c.Cipher}
	srv, err = asheClient.WithCipher(&daze.Context{Cid: "00000000"}, srv)
	if err != nil {
		log.Println(err)
		goto Tag1
	}
	srv = NewLioConn(srv)

	go func() {
		for {
			select {
			case c.Srv <- srv:
			case <-closedChan:
				return
			}
		}
	}()

	var (
		buf    = make([]byte, 2048)
		cmd    uint8
		idx    uint16
		msgLen uint16
		ok     bool
		sio    *SioConn
	)
	for {
		_, err = io.ReadFull(srv, buf[:8])
		if err != nil {
			break
		}
		if Conf.LogClient != 0 {
			log.Printf("%s   recv data=[% x]", "cccccccc", buf[:8])
		}
		cmd = buf[0]
		switch cmd {
		case 1:
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			doa.Doa(msgLen <= 2040)
			buf[0] = 2
			srv.Write(buf[:8+msgLen])
		case 2:
			idx = binary.BigEndian.Uint16(buf[1:3])
			msgLen = binary.BigEndian.Uint16(buf[3:5])
			doa.Doa(int(msgLen) <= 2040)
			_, err = io.ReadFull(srv, buf[:msgLen])
			if err != nil {
				break
			}
			sio, ok = c.Harbor[idx]
			if ok {
				sio.ReaderWriter.Write(buf[0:msgLen])
			}
		case 3:
			idx = binary.BigEndian.Uint16(buf[1:3])
			sio, ok = c.Harbor[idx]
			doa.Doa(ok)
			if ok {
				sio.ReaderWriter.Write(buf[:8])
			}
		case 4:
			idx = binary.BigEndian.Uint16(buf[1:3])
			sio, ok = c.Harbor[idx]
			doa.Doa(ok)
			if ok {
				sio.CloseOther()
			}
		}
	}

	for _, sio := range c.Harbor {
		sio.CloseOther()
	}

	closedChan <- 0
	goto Tag1
}

// Close the static link.
func (c *Client) Close() error {
	close(c.Closed)
	select {
	case srv := <-c.Srv:
		return srv.Close()
	default:
		return nil
	}
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher, as a sign to interface with the Server.
func NewClient(server, cipher string) *Client {
	idpool := make(chan uint16, 256)
	for i := uint16(1); i < 256; i++ {
		idpool <- i
	}
	client := &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
		Closed: make(chan int),
		Harbor: map[uint16]*SioConn{},
		IDPool: idpool,
		Srv:    make(chan io.ReadWriteCloser, 1),
	}
	go client.Serve()
	return client
}
