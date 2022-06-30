package crow

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"sync/atomic"

	"github.com/mohanson/daze"
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
// +-----+-----+    +-----+-----+-----+
// |  1  | Len |    | Rsv | Len | Msg |
// +-----+-----+    +-----+-----+-----+
// |  1  |  2  |    |  1  |  2  |  N  |
// +-----+-----+    +-----+-----+-----+
//
// Both server and client can push data to each other. The ID in command can be obtained in next command.
//
// +-----+-----+-----+-----+
// |  2  | ID  | Len | Msg |
// +-----+-----+-----+-----+
// |  1  |  2  |  2  |  N  |
// +-----+-----+-----+-----+
//
// Client wishes to establish a connection. The client needs to transmit two network and destination address. The server
// will typically evaluate the request based on network and destination addresses, and return one reply messages, as
// appropriate for the request type.
//
// +-----+-----+-----+-----+    +-----+-----+
// |  3  | Net | Len | Dst |    | Rep | ID  |
// +-----+-----+-----+-----+    +-----+-----+
// |  1  |  1  |  2  |  N  |    |  1  |  2  |
// +-----+-----+-----+-----+    +-----+-----+
//
// Net: 0x01 TCP
//      0x02 UDP
// Rep: 0x00 succeeded
//      0x01 general failure
//
// Close the specified connection.
//
// +-----+-----+
// |  4  | ID  |
// +-----+-----+
// |  1  |  2  |
// +-----+-----+

// Server implemented the crow protocol.
type Server struct {
	Listen string
	Cipher [16]byte
	Closer io.Closer
}

// Serve. Parameter raw will be closed automatically when the function exits.
func (s *Server) Serve(ctx *daze.Context, raw io.ReadWriteCloser) error {
	var (
		buf = make([]byte, 128)
		cli io.ReadWriteCloser
		len uint16
		err error
	)
	_, err = io.ReadFull(raw, buf[:128])
	if err != nil {
		return err
	}
	cli = daze.Gravity(raw, append(buf[:128], s.Cipher[:]...))

	for {
		_, err = cli.Read(buf[:1])
		if err != nil {
			break
		}
		switch buf[0] {
		case 1:
			cli.Read(buf[:2])
			len = binary.BigEndian.Uint16(buf[:2])
			io.CopyN(cli, daze.Conf.Random, int64(len))
		case 2:
		case 3:
		case 4:
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

	i := uint32(math.MaxUint32)
	for {
		cli, err := ln.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Println(err)
			}
			break
		}
		go func(cli net.Conn) {
			defer cli.Close()
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, atomic.AddUint32(&i, 1))
			cid := hex.EncodeToString(buf)
			ctx := &daze.Context{Cid: cid}
			log.Printf("%s accept remote=%s", cid, cli.RemoteAddr())
			if err := s.Serve(ctx, cli); err != nil {
				log.Println(cid, " error", err)
			}
			log.Println(cid, "closed")
		}(cli)
	}
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
	Conn   io.ReadWriteCloser
}

// Deal with crow protocol. It is the caller's responsibility to close the srv.
func (c *Client) DialDaze(ctx *daze.Context, srv io.ReadWriteCloser, network string, address string) (io.ReadWriteCloser, error) {
	var (
		n   = len(address)
		buf = make([]byte, 128)
	)
	if n > 255 {
		return nil, fmt.Errorf("daze: destination address too long %s", address)
	}
	if network != "tcp" && network != "udp" {
		return nil, fmt.Errorf("daze: network must be tcp or udp")
	}
	daze.Conf.Random.Read(buf[:128])
	srv.Write(buf[:128])
	srv = daze.Gravity(srv, append(buf[:128], c.Cipher[:]...))
	return srv, nil
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	srv, err := daze.Conf.Dialer.Dial("tcp", c.Server)
	if err != nil {
		return nil, err
	}
	ret, err := c.DialDaze(ctx, srv, network, address)
	if err != nil {
		srv.Close()
	}
	return ret, err
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher, as a sign to interface with the Server.
func NewClient(server, cipher string) *Client {
	c := &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
	}
	buf := make([]byte, 128)
	srv, err := daze.Conf.Dialer.Dial("tcp", c.Server)
	if err != nil {
		return nil
	}
	daze.Conf.Random.Read(buf[:128])
	srv.Write(buf[:128])
	c.Conn = daze.Gravity(srv, append(buf[:128], c.Cipher[:]...))
	return c
}
