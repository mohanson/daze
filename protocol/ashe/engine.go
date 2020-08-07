package ashe

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"sync/atomic"
	"time"

	"github.com/mohanson/daze"
)

// This document specifies an Internet protocol for the Internet community. For traverse a firewall transparently and
// securely, ASHE used rc4 encryption with one-time password. In order to fight replay attacks, ASHE get inspiration
// from cookie, added a timestamp inside the frame.
//
// The client connects to the server, and sends a version identifier/method selection message:
//
// +-----+-----------+------+-----+---------+---------+
// | OTA | Handshake | Time | Net | DST.Len | DST     |
// +-----+-----------+------+-----+---------+---------+
// | 128 | 2         | 8    |  1  | 1       | 0 - 255 |
// +-----+-----------+------+-----+---------+---------+
//
// - OTA: random 128 bytes for rc4 key
// - Handshake: must be 0xFF, 0xFF
// - Time: timestamp of request
// - Net: tcp(0x01), udp(0x03)
// - DST.Len: len of DST. If DST is https://google.com, DST.Len is 0x12
// - DST: desired destination address

// TCPConn is an implementation of the Conn interface for TCP network connections.
type TCPConn struct {
	io.ReadWriteCloser
}

// UDPConn is the implementation of the Conn and PacketConn interfaces for UDP network connections.
type UDPConn struct {
	io.ReadWriteCloser
}

// Close closes the connection.
func (c *UDPConn) Close() error {
	return c.ReadWriteCloser.Close()
}

// Read implements the Conn Read method.
func (c *UDPConn) Read(p []byte) (int, error) {
	_, err := io.ReadFull(c.ReadWriteCloser, p[:4])
	if err != nil {
		return 0, err
	}
	n := binary.BigEndian.Uint32(p[:4])
	return io.ReadFull(c.ReadWriteCloser, p[:n])
}

// Write implements the Conn Write method.
func (c *UDPConn) Write(p []byte) (int, error) {
	if len(p) > math.MaxUint16 {
		panic("unreachable")
	}
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(len(p)))
	c.ReadWriteCloser.Write(b)
	return c.ReadWriteCloser.Write(p)
}

// Server implemented the ashe protocol. The ASHE server will typically evaluate the request based on source and
// destination addresses, and return one or more reply messages, as appropriate for the request type.
type Server struct {
	Listen string
	Cipher [16]byte
}

// Serve.
func (s *Server) Serve(ctx context.Context, cli io.ReadWriteCloser) error {
	var (
		buf = make([]byte, 1024)
		dst string
		srv io.ReadWriteCloser
		err error
	)
	_, err = io.ReadFull(cli, buf[:128])
	if err != nil {
		return err
	}
	cli = daze.Gravity(cli, append(buf[:128], s.Cipher[:]...))

	_, err = io.ReadFull(cli, buf[:12])
	if err != nil {
		return err
	}
	if buf[0] != 0xff || buf[1] != 0xff {
		return fmt.Errorf("daze: malformed request: [%# 02x]", buf[0:2])
	}
	d := int64(binary.BigEndian.Uint64(buf[2:10]))
	if math.Abs(float64(time.Now().Unix()-d)) > 120 {
		return fmt.Errorf("daze: expired: %v", time.Unix(d, 0))
	}
	_, err = io.ReadFull(cli, buf[12:12+buf[11]])
	if err != nil {
		return err
	}
	dst = string(buf[12 : 12+buf[11]])
	switch buf[10] {
	case 0x01:
		log.Printf("%s   dial network=tcp address=%s", ctx.Value("cid"), dst)
		srv, err = net.DialTimeout("tcp", dst, time.Second*4)
		cli = &TCPConn{cli}
	case 0x03:
		log.Printf("%s   dial network=udp address=%s", ctx.Value("cid"), dst)
		srv, err = net.DialTimeout("udp", dst, time.Second*4)
		cli = &UDPConn{cli}
	}
	if err != nil {
		return err
	}
	defer srv.Close()
	daze.Link(cli, srv)
	return nil
}

// Run.
func (s *Server) Run() error {
	ln, err := net.Listen("tcp", s.Listen)
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Println("listen and serve on", s.Listen)

	i := uint32(math.MaxUint32)
	for {
		cli, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go func(cli net.Conn) {
			defer cli.Close()
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, atomic.AddUint32(&i, 1))
			cid := hex.EncodeToString(buf)
			ctx := context.WithValue(context.Background(), "cid", cid)
			log.Printf("%s accept remote=%s", cid, cli.RemoteAddr())
			if err := s.Serve(ctx, cli); err != nil {
				log.Println(cid, " error", err)
			}
			log.Println(cid, "closed")
		}(cli)
	}
}

// NewServer returns a new Server. A secret data needs to be passed in Cipher, as a sign to interface with the Client.
func NewServer(listen string, cipher string) *Server {
	return &Server{
		Listen: listen,
		Cipher: md5.Sum([]byte(cipher)),
	}
}

// Client implemented the ashe protocol.
type Client struct {
	Server string
	Cipher [16]byte
}

// Dial. It is similar to the server, the only difference is that it constructs the data and the server parses the
// data. This code I refer to the golang socks5 official library. That is a good code which is opened with expectation,
// and closed with delight and profit.
func (c *Client) Dial(ctx context.Context, network string, address string) (io.ReadWriteCloser, error) {
	log.Printf("%s   dial routing=client network=%s address=%s", ctx.Value("cid"), network, address)
	var (
		srv io.ReadWriteCloser
		n   = len(address)
		buf = make([]byte, 512)
		err error
	)
	if n > 256 {
		return nil, fmt.Errorf("daze: destination address too long: %s", address)
	}
	if network != "tcp" && network != "udp" {
		return nil, fmt.Errorf("daze: network must be tcp or udp, but get %s", network)
	}
	srv, err = net.DialTimeout("tcp", c.Server, time.Second*4)
	if err != nil {
		return nil, err
	}
	rand.Read(buf[:128])
	srv.Write(buf[:128])
	srv = daze.Gravity(srv, append(buf[:128], c.Cipher[:]...))
	buf[0x00] = 0xff
	buf[0x01] = 0xff
	binary.BigEndian.PutUint64(buf[2:10], uint64(time.Now().Unix()))
	switch network {
	case "tcp":
		buf[0x0a] = 0x01
	case "udp":
		buf[0x0a] = 0x03
	}
	buf[0x0b] = uint8(n)
	copy(buf[12:], []byte(address))
	_, err = srv.Write(buf[:12+n])
	if err != nil {
		return nil, err
	}
	switch network {
	case "tcp":
		return &TCPConn{srv}, nil
	case "udp":
		return &UDPConn{srv}, nil
	}
	return nil, nil
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher, as a sign to interface with the Server.
func NewClient(server, cipher string) *Client {
	return &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
	}
}
