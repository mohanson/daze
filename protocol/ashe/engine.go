package ashe

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"time"

	"github.com/mohanson/daze"
)

// This document specifies an Internet protocol for the Internet community.
// For traverse a firewall transparently and securely, ASHE used
// rc4 encryption with one-time password. In order to fight replay attacks,
// ASHE get inspiration from cookie, added a timestamp inside the frame.
//
// The client connects to the server, and sends a version identifier/method
// selection message:
//
// +-----+-----------+------+-----+---------+---------+
// | OTA | Handshake | Time | RSV | DST.Len | DST     |
// +-----+-----------+------+-----+---------+---------+
// | 128 | 2         | 8    |  1  | 1       | 0 - 255 |
// +-----+-----------+------+-----+---------+---------|
//
// - OTA: random 128 bytes for rc4 key
// - Handshake: must be 0xFF, 0xFF
// - Time: timestamp of request
// - RSV: reserved
// - DST.Len: len of DST. If DST is https://google.com, DST.Len is 0x12
// - DST: desired destination address

// Server implemented the ashe protocol. The ASHE server will typically
// evaluate the request based on source and destination addresses, and return
// one or more reply messages, as appropriate for the request type.
type Server struct {
	Listen string
	Cipher [16]byte
}

// Serve.
func (s *Server) Serve(cli io.ReadWriteCloser) error {
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
	log.Println("connect[ashe]", dst)
	switch buf[10] {
	case 0x01:
		srv, err = net.DialTimeout("tcp", dst, time.Second*4)
	case 0x03:
		srv, err = net.DialTimeout("udp", dst, time.Second*4)
	default:
		return fmt.Errorf("daze: the network must be tcp or udp")
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

	for {
		cli, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go func() {
			defer cli.Close()
			if err := s.Serve(cli); err != nil {
				log.Println(err)
			}
		}()
	}
}

// NewServer returns a new Server. A secret data needs to be passed in Cipher,
// as a sign to interface with the Client.
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

func (c *Client) DialConn(srv io.ReadWriteCloser, network string, address string) (io.ReadWriteCloser, error) {
	var (
		buf = make([]byte, 512)
		n   = len(address)
		err error
	)
	if n > 256 {
		return nil, fmt.Errorf("daze: destination address too long: %s", address)
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
	default:
		return nil, fmt.Errorf("daze: the network must be tcp or udp")
	}
	buf[0x0b] = uint8(n)
	copy(buf[12:], []byte(address))
	_, err = srv.Write(buf[:12+n])
	if err != nil {
		return nil, err
	}
	return srv, nil
}

// Dial. It is similar to the server, the only difference is that it constructs
// the data and the server parses the data. This code I refer to the golang
// socks5 official library. That is a good code which is opened with
// expectation, and closed with delight and profit.
func (c *Client) Dial(network string, address string) (io.ReadWriteCloser, error) {
	srv, err := net.DialTimeout("tcp", c.Server, time.Second*4)
	if err != nil {
		return nil, err
	}
	return c.DialConn(srv, network, address)
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher,
// as a sign to interface with the Server.
func NewClient(server, cipher string) *Client {
	return &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
	}
}
