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
	"os"
	"sync/atomic"

	"github.com/mohanson/daze"
)

// Server implemented the crow protocol.
type Server struct {
	Listen string
	Cipher [16]byte
	Closer io.Closer
}

// Serve. Parameter raw will be closed automatically when the function exits.
func (s *Server) Serve(ctx *daze.Context, raw io.ReadWriteCloser) error {
	var (
		buf = make([]byte, 256)
		cli io.ReadWriteCloser
		err error
	)
	_, err = io.ReadFull(raw, buf[:128])
	if err != nil {
		return err
	}
	cli = daze.Gravity(raw, append(buf[:128], s.Cipher[:]...))

	io.Copy(os.Stdout, cli)
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
}

// Deal with crow protocol. It is the caller's responsibility to close the srv.
func (c *Client) Deal(ctx *daze.Context, srv io.ReadWriteCloser, network string, address string) (io.ReadWriteCloser, error) {
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
	ret, err := c.Deal(ctx, srv, network, address)
	if err != nil {
		srv.Close()
	}
	return ret, err
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher, as a sign to interface with the Server.
func NewClient(server, cipher string) *Client {
	return &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
	}
}
