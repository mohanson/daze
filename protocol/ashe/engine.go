package ashe

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"time"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
)

// This document describes a tcp-based cryptographic proxy protocol. The main purpose of this protocol is to bypass
// firewalls while providing a good user experience, so it only provides minimal security, which is one of the reasons
// for choosing the rc4 algorithm(rc4 is cryptographically broken and should not be used for secure applications).
//
// The client connects to the server, and sends a request details:
//
// +------+------+-----+---------+---------+
// | Salt | Time | Net | Dst.Len | Dst     |
// +------+------+-----+---------+---------+
// | 128  | 8    | 1   | 1       | 0 - 255 |
// +------+------+-----+---------+---------+
//
// - Salt    : Random 128 bytes for rc4 key, all data will be transmitted encrypted after there
// - Time    : Timestamp of request. The server will reject requests with past or future timestamps to prevent replay
//             attacks
// - Net     : 0x01 : TCP
//             0x03 : UDP
// - Dst.Len : Destination address's length
// - Dst     : Destination address
//
// The server returns:
//
// +------+
// | Code |
// +------+
// |  1   |
// +------+
//
// - Code: 0x00: Succeed
//         0x01: General server failure

// Conf is acting as package level configuration.
var Conf = struct {
	// The time error allowed by the server in seconds.
	LifeExpired int
}{
	LifeExpired: 120,
}

// TCPConn is an implementation of the Conn interface for tcp network connections.
type TCPConn struct {
	io.ReadWriteCloser
}

// NewTCPConn returns a new TCPConn.
func NewTCPConn(c io.ReadWriteCloser) *TCPConn {
	return &TCPConn{c}
}

// UDPConn is an implementation of the Conn interface for udp network connections.
type UDPConn struct {
	io.ReadWriteCloser
}

// NewUDPConn returns a new UDPConn.
func NewUDPConn(c io.ReadWriteCloser) *UDPConn {
	return &UDPConn{ReadWriteCloser: c}
}

// Read reads up to len(p) bytes into p.
func (c *UDPConn) Read(p []byte) (int, error) {
	doa.Doa(len(p) >= 2)
	_, err := io.ReadFull(c.ReadWriteCloser, p[:2])
	if err != nil {
		return 0, err
	}
	n := int(binary.BigEndian.Uint16(p[:2]))
	doa.Doa(len(p) >= n)
	return io.ReadFull(c.ReadWriteCloser, p[:n])
}

// Write writes len(p) bytes from p to the underlying data stream.
func (c *UDPConn) Write(p []byte) (int, error) {
	// Maximum udp payload size is 65527(equal to 65535 - 8) bytes in theoretically. The 8 in the formula means the udp
	// header, which contains source port, destination port, length and checksum.
	doa.Doa(len(p) <= 65527)
	b := make([]byte, 2+len(p))
	binary.BigEndian.PutUint16(b, uint16(len(p)))
	copy(b[2:], p)
	n, err := c.ReadWriteCloser.Write(b)
	if err != nil {
		return 0, err
	}
	return n - 2, nil
}

// Server implemented the ashe protocol. The ashe server will typically evaluate the request based on source and
// destination addresses, and return one or more reply messages, as appropriate for the request type.
type Server struct {
	// Cipher is a pre-shared key.
	Cipher []byte
	Closer io.Closer
	Listen string
}

// Hello creates an encrypted channel.
func (s *Server) Hello(cli io.ReadWriteCloser) (io.ReadWriteCloser, error) {
	var (
		buf     = make([]byte, 32)
		con     io.ReadWriteCloser
		err     error
		gap     int64
		gapSign int64
	)
	_, err = io.ReadFull(cli, buf[:])
	if err != nil {
		return nil, err
	}
	// To build a key from pre-shared key. Use xor as our key derivation function.
	for i := range 32 {
		buf[i] ^= s.Cipher[i]
	}
	con = daze.Gravity(cli, buf[:])
	_, err = io.ReadFull(con, buf[:8])
	if err != nil {
		return nil, err
	}
	// Get absolute value. Hacker's Delight, 2-4, Absolute Value Function.
	// See https://doc.lagout.org/security/Hackers%20Delight.pdf
	gap = time.Now().Unix() - int64(binary.BigEndian.Uint64(buf[:8]))
	gapSign = gap >> 63
	if gap^gapSign-gapSign > int64(Conf.LifeExpired) {
		return nil, errors.New("daze: request expired")
	}
	return con, nil
}

// Serve incoming connections. Parameter cli will be closed automatically when the function exits.
func (s *Server) Serve(ctx *daze.Context, cli io.ReadWriteCloser) error {
	var (
		buf    = make([]byte, 256)
		con    io.ReadWriteCloser
		dst    string
		dstLen uint8
		dstNet uint8
		err    error
		srv    io.ReadWriteCloser
	)
	con, err = s.Hello(cli)
	if err != nil {
		return err
	}
	_, err = io.ReadFull(con, buf[:2])
	if err != nil {
		return err
	}
	dstNet = buf[0]
	dstLen = buf[1]
	_, err = io.ReadFull(con, buf[:dstLen])
	if err != nil {
		return err
	}
	dst = string(buf[:dstLen])
	switch dstNet {
	case 0x01:
		log.Printf("conn: %08x   dial network=tcp address=%s", ctx.Cid, dst)
		srv, err = daze.Dial("tcp", dst)
	case 0x03:
		log.Printf("conn: %08x   dial network=udp address=%s", ctx.Cid, dst)
		srv, err = daze.Dial("udp", dst)
	}
	if err != nil {
		con.Write([]byte{1})
		return err
	}
	con.Write([]byte{0})
	switch dstNet {
	case 0x01:
		con = NewTCPConn(con)
	case 0x03:
		con = NewUDPConn(con)
	}
	daze.Link(con, srv)
	return nil
}

// Close listener. Established connections will not be closed.
func (s *Server) Close() error {
	if s.Closer != nil {
		return s.Closer.Close()
	}
	return nil
}

// Run it.
func (s *Server) Run() error {
	l, err := net.Listen("tcp", s.Listen)
	if err != nil {
		return err
	}
	s.Closer = l
	log.Println("main: listen and serve on", s.Listen)

	go func() {
		idx := uint32(math.MaxUint32)
		for {
			cli, err := l.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Println("main:", err)
				}
				break
			}
			idx++
			ctx := &daze.Context{Cid: idx}
			log.Printf("conn: %08x accept remote=%s", ctx.Cid, cli.RemoteAddr())
			go func() {
				defer cli.Close()
				if err := s.Serve(ctx, cli); err != nil {
					log.Printf("conn: %08x  error %s", ctx.Cid, err)
				}
				log.Printf("conn: %08x closed", ctx.Cid)
			}()
		}
	}()

	return nil
}

// NewServer returns a new Server. Cipher is a password in string form, with no length limit.
func NewServer(listen string, cipher string) *Server {
	return &Server{
		Listen: listen,
		Cipher: daze.Salt(cipher),
	}
}

// Client implemented the ashe protocol.
type Client struct {
	// Cipher is a pre-shared key.
	Cipher []byte
	Server string
}

// Hello creates an encrypted channel.
func (c *Client) Hello(srv io.ReadWriteCloser) (io.ReadWriteCloser, error) {
	var (
		buf = make([]byte, 32)
		con io.ReadWriteCloser
		err error
	)
	io.ReadFull(&daze.RandomReader{}, buf[:])
	_, err = srv.Write(buf[:])
	if err != nil {
		return nil, err
	}
	// To build a key from pre-shared key. Use xor as our key derivation function.
	for i := range 32 {
		buf[i] ^= c.Cipher[i]
	}
	con = daze.Gravity(srv, buf[:])
	binary.BigEndian.PutUint64(buf[:8], uint64(time.Now().Unix()))
	_, err = con.Write(buf[:8])
	if err != nil {
		return nil, err
	}
	return con, nil
}

// Establish an existing connection. It is the caller's responsibility to close the conn.
func (c *Client) Estab(ctx *daze.Context, srv io.ReadWriteCloser, network string, address string) (io.ReadWriteCloser, error) {
	var (
		buf = make([]byte, 2)
		con io.ReadWriteCloser
		err error
		n   = len(address)
	)
	if n > 255 {
		return nil, fmt.Errorf("daze: destination address too long %s", address)
	}
	if network != "tcp" && network != "udp" {
		return nil, fmt.Errorf("daze: network must be tcp or udp")
	}
	con, err = c.Hello(srv)
	if err != nil {
		return nil, err
	}
	switch network {
	case "tcp":
		buf[0x00] = 0x01
	case "udp":
		buf[0x00] = 0x03
	}
	buf[0x01] = uint8(n)
	con.Write(buf[:2])
	_, err = con.Write([]byte(address))
	if err != nil {
		return nil, err
	}
	_, err = io.ReadFull(con, buf[:1])
	if err != nil {
		return nil, err
	}
	switch {
	case buf[0] == 0:
	case buf[0] == 1:
		return nil, errors.New("daze: general server failure")
	case buf[0] >= 2:
		return nil, errors.New("daze: receive error response")
	}
	switch network {
	case "tcp":
		return NewTCPConn(con), nil
	case "udp":
		return NewUDPConn(con), nil
	}
	panic("unreachable")
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	srv, err := daze.Dial("tcp", c.Server)
	if err != nil {
		return nil, err
	}
	con, err := c.Estab(ctx, srv, network, address)
	if err != nil {
		srv.Close()
	}
	return con, err
}

// NewClient returns a new Client. Cipher is a password in string form, with no length limit.
func NewClient(server, cipher string) *Client {
	return &Client{
		Server: server,
		Cipher: daze.Salt(cipher),
	}
}
