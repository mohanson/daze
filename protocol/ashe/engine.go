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
// +-----+-----------+------+-----+---------+---------|
// | 128 | 2         | 8    |  1  | 1       | 0 - 255 |
// +-----+-----------+------+-----+---------+---------|
//
// - OTA: random 128 bytes for rc4 key
// - Handshake: must be 0xFF, 0xFF
// - Time: Timestamp of request
// - RSV: Reserved
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
func (s *Server) Serve(connl io.ReadWriteCloser) error {
	var (
		buf   = make([]byte, 1024)
		dst   string
		connr io.ReadWriteCloser
		err   error
	)
	killer := time.AfterFunc(time.Second*8, func() {
		connl.Close()
	})
	defer killer.Stop()

	_, err = io.ReadFull(connl, buf[:128])
	if err != nil {
		return err
	}
	connl = daze.Gravity(connl, append(buf[:128], s.Cipher[:]...))

	_, err = io.ReadFull(connl, buf[:12])
	if err != nil {
		return err
	}
	if buf[0] != 0xFF || buf[1] != 0xFF {
		return fmt.Errorf("daze: malformed request: %v", buf[:2])
	}
	d := int64(binary.BigEndian.Uint64(buf[2:10]))
	if math.Abs(float64(time.Now().Unix()-d)) > 120 {
		return fmt.Errorf("daze: expired: %v", time.Unix(d, 0))
	}
	_, err = io.ReadFull(connl, buf[12:12+buf[11]])
	if err != nil {
		return err
	}
	killer.Stop()
	dst = string(buf[12 : 12+buf[11]])
	log.Println("Connect[ashe]", dst)
	if buf[10] == 0x03 {
		connr, err = net.DialTimeout("udp", dst, time.Second*8)
	} else {
		connr, err = net.DialTimeout("tcp", dst, time.Second*8)
	}
	if err != nil {
		return err
	}
	defer connr.Close()

	daze.Link(connl, connr)
	return nil
}

// Run.
func (s *Server) Run() error {
	ln, err := net.Listen("tcp", s.Listen)
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Println("Listen and serve on", s.Listen)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go func() {
			defer conn.Close()
			if err := s.Serve(conn); err != nil {
				log.Println(err)
			}
		}()
	}
}

// NewServer returns a new Server. A secret data needs to be passed in Cipher,
// as a sign to interface with the Client.
func NewServer(listen, cipher string) *Server {
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

// Dial. It is similar to the server, the only difference is that it constructs
// the data and the server parses the data. This code I refer to the golang
// socks5 official library. That is a good code which is opened with
// expectation, and closed with delight and profit.
func (c *Client) Dial(network, address string) (io.ReadWriteCloser, error) {
	var (
		conn io.ReadWriteCloser
		buf  = make([]byte, 1024)
		err  error
		n    = len(address)
	)
	if n > 256 {
		return nil, fmt.Errorf("daze: destination address too long: %s", address)
	}
	conn, err = net.DialTimeout("tcp", c.Server, time.Second*8)
	if err != nil {
		return nil, err
	}
	rand.Read(buf[:128])
	conn.Write(buf[:128])
	conn = daze.Gravity(conn, append(buf[:128], c.Cipher[:]...))

	buf[0] = 0xFF
	buf[1] = 0xFF
	binary.BigEndian.PutUint64(buf[2:10], uint64(time.Now().Unix()))
	switch network {
	case "tcp", "tcp4", "tcp6":
		buf[10] = 0x01
	case "udp", "udp4", "udp6":
		buf[10] = 0x03
	}
	buf[11] = uint8(n)
	copy(buf[12:], []byte(address))
	conn.Write(buf[:12+n])
	return conn, nil
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher,
// as a sign to interface with the Server.
func NewClient(server, cipher string) *Client {
	return &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
	}
}
