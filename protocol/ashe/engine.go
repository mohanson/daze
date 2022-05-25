package ashe

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"sync/atomic"
	"time"

	"github.com/godump/doa"
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
// - Handshake: must be 0xff, 0xff
// - Time: timestamp of request
// - Net: tcp(0x01), udp(0x03)
// - DST.Len: len of DST. If DST is https://google.com, DST.Len is 0x12
// - DST: desired destination address
//
// The server returns:
//
// +------+
// | Code |
// +------+
// |  1   |
// +------+
//
// - Code: 0x00: succeed
//         0x01: general server failure

var Conf = struct {
	LifeExpired int
}{
	LifeExpired: 120,
}

// TCPConn is an implementation of the Conn interface for TCP network connections.
type TCPConn struct {
	io.ReadWriteCloser
}

// UDPConn is an implementation of the Conn interface for UDP network connections.
type UDPConn struct {
	io.ReadWriteCloser
}

// Read implements the Conn Read method.
func (c *UDPConn) Read(p []byte) (int, error) {
	_, err := io.ReadFull(c.ReadWriteCloser, p[:2])
	if err != nil {
		return 0, err
	}
	n := binary.BigEndian.Uint16(p[:2])
	return io.ReadFull(c.ReadWriteCloser, p[:n])
}

// Write implements the Conn Write method.
func (c *UDPConn) Write(p []byte) (int, error) {
	// Maximum UDP packet size is 2^16 bytes in theoretically.
	// But every packet lives in an Ethernet frame. Ethernet frames can only contain 1500 bytes of data. This is called
	// the "maximum transmission unit" or "MTU".
	doa.Doa(len(p) <= math.MaxUint16)
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(len(p)))
	_, err := c.ReadWriteCloser.Write(b)
	if err != nil {
		return 0, err
	}
	return c.ReadWriteCloser.Write(p)
}

// Server implemented the ashe protocol. The ASHE server will typically evaluate the request based on source and
// destination addresses, and return one or more reply messages, as appropriate for the request type.
type Server struct {
	Listen string
	Cipher [16]byte
}

// Serve.
func (s *Server) Serve(ctx *daze.Context, raw io.ReadWriteCloser) error {
	var (
		buf    = make([]byte, 256)
		cli    io.ReadWriteCloser
		dst    string
		dstLen uint8
		dstNet uint8
		srv    io.ReadWriteCloser
		err    error
	)
	_, err = io.ReadFull(raw, buf[:128])
	if err != nil {
		return err
	}
	cli = daze.Gravity(raw, append(buf[:128], s.Cipher[:]...))

	_, err = io.ReadFull(cli, buf[:12])
	if err != nil {
		return err
	}
	if buf[0] != 0xff || buf[1] != 0xff {
		return errors.New("daze: request malformed")
	}
	d := time.Now().Unix() - int64(binary.BigEndian.Uint64(buf[2:10]))
	y := d >> 63
	if d^y-y > int64(Conf.LifeExpired) {
		return errors.New("daze: request expired")
	}
	dstNet = buf[10]
	dstLen = buf[11]
	_, err = io.ReadFull(cli, buf[:dstLen])
	if err != nil {
		return err
	}
	dst = string(buf[:dstLen])
	switch dstNet {
	case 0x01:
		log.Printf("%s   dial network=tcp address=%s", ctx.Cid, dst)
		srv, err = daze.Conf.Dialer.Dial("tcp", dst)
	case 0x03:
		log.Printf("%s   dial network=udp address=%s", ctx.Cid, dst)
		srv, err = daze.Conf.Dialer.Dial("udp", dst)
	}
	if err != nil {
		cli.Write([]byte{1})
		return err
	}
	cli.Write([]byte{0})
	switch dstNet {
	case 0x01:
		cli = &TCPConn{cli}
	case 0x03:
		cli = &UDPConn{cli}
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
			continue
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
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	var (
		srv io.ReadWriteCloser
		n   = len(address)
		buf = make([]byte, 128)
		err error
	)
	if n > 255 {
		return nil, fmt.Errorf("daze: destination address too long %s", address)
	}
	if network != "tcp" && network != "udp" {
		return nil, fmt.Errorf("daze: network must be tcp or udp")
	}
	srv, err = daze.Conf.Dialer.Dial("tcp", c.Server)
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
	srv.Write(buf[:12])
	_, err = srv.Write([]byte(address))
	if err != nil {
		return nil, err
	}
	_, err = io.ReadFull(srv, buf[:1])
	if err != nil {
		return nil, err
	}
	if buf[0] != 0 {
		return nil, errors.New("daze: general server failure")
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
