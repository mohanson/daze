package ashe

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/mohanson/daze"
)

type Server struct {
	Listen string
	Cipher [16]byte
}

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
	log.Println("Connect", dst)
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
		go func(conn net.Conn) {
			defer conn.Close()
			if err := s.Serve(conn); err != nil {
				log.Println(err)
			}
		}(conn)
	}
}

func NewServer(listen, cipher string) *Server {
	return &Server{
		Listen: listen,
		Cipher: md5.Sum([]byte(cipher)),
	}
}

type Client struct {
	Server string
	Cipher [16]byte
}

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
	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	rand.Read(buf[:128])
	_, err = conn.Write(buf[:128])
	if err != nil {
		return nil, err
	}
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
	_, err = conn.Write(buf[:12+n])
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func NewClient(server, cipher string) *Client {
	return &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
	}
}

type Locale struct {
	Listen string
	Dialer daze.Dialer
}

func (l *Locale) ServeProxy(connl io.ReadWriteCloser) error {
	reader := bufio.NewReader(connl)
	r, err := http.ReadRequest(reader)
	if err != nil {
		return err
	}

	var port string
	if r.URL.Port() == "" {
		port = "80"
	} else {
		port = r.URL.Port()
	}
	log.Println("Connect", r.URL.Hostname()+":"+port)

	connr, err := l.Dialer.Dial("tcp", r.URL.Hostname()+":"+port)
	if err != nil {
		return err
	}

	if r.Method == "CONNECT" {
		if _, err := connl.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
			return err
		}
	} else if r.Method == "GET" && r.Header.Get("Upgrade") == "websocket" {
		if err := r.Write(connr); err != nil {
			return err
		}
	} else {
		r.Header.Set("Connection", "close")
		if err := r.Write(connr); err != nil {
			return err
		}
	}

	daze.Link(connl, connr)
	return nil
}

func (l *Locale) ServeSocks(connl io.ReadWriteCloser) error {
	var (
		buf        = make([]byte, 1024)
		n          int
		dstNetwork uint8
		dstCase    uint8
		dstHost    string
		dstPort    uint16
		dst        string
		connr      io.ReadWriteCloser
		err        error
	)

	_, err = io.ReadFull(connl, buf[:2])
	if err != nil {
		return err
	}
	n = int(buf[1])
	_, err = io.ReadFull(connl, buf[:n])
	if err != nil {
		return err
	}
	_, err = connl.Write([]byte{0x05, 0x00})
	if err != nil {
		return err
	}

	_, err = io.ReadFull(connl, buf[:4])
	if err != nil {
		return err
	}
	dstNetwork = buf[1]
	dstCase = buf[3]
	switch dstCase {
	case 0x01:
		_, err = io.ReadFull(connl, buf[:4])
		if err != nil {
			return err
		}
		dstHost = net.IP(buf[:4]).String()
	case 0x03:
		_, err = io.ReadFull(connl, buf[:1])
		if err != nil {
			return err
		}
		n = int(buf[0])
		_, err = io.ReadFull(connl, buf[:n])
		if err != nil {
			return err
		}
		dstHost = string(buf[:n])
	case 0x04:
		_, err = io.ReadFull(connl, buf[:16])
		if err != nil {
			return err
		}
		dstHost = net.IP(buf[:16]).String()
	}
	_, err = io.ReadFull(connl, buf[:2])
	if err != nil {
		return err
	}
	dstPort = binary.BigEndian.Uint16(buf[:2])
	dst = dstHost + ":" + strconv.Itoa(int(dstPort))
	log.Println("Connect", dst)

	if dstNetwork == 0x03 {
		connr, err = l.Dialer.Dial("udp", dst)
	} else {
		connr, err = l.Dialer.Dial("tcp", dst)
	}
	if err != nil {
		connl.Write([]byte{0x05, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return err
	}
	connl.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	defer connr.Close()

	daze.Link(connl, connr)
	return nil
}

func (l *Locale) Serve(connl io.ReadWriteCloser) error {
	var (
		buf = make([]byte, 1)
		err error
	)
	_, err = io.ReadFull(connl, buf)
	if err != nil {
		return err
	}
	connl = daze.ReadWriteCloser{
		Reader: io.MultiReader(bytes.NewReader(buf), connl),
		Writer: connl,
		Closer: connl,
	}
	if buf[0] == 0x05 {
		err = l.ServeSocks(connl)
	} else {
		err = l.ServeProxy(connl)
	}
	return err
}

func (l *Locale) Run() error {
	ln, err := net.Listen("tcp", l.Listen)
	if err != nil {
		return err
	}
	defer ln.Close()
	log.Println("Listen and serve on", l.Listen)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go func(conn net.Conn) {
			defer conn.Close()
			if err := l.Serve(conn); err != nil {
				log.Println(err)
			}
		}(conn)
	}
}

func NewLocale(listen string, dialer daze.Dialer) *Locale {
	return &Locale{
		Listen: listen,
		Dialer: dialer,
	}
}

type Filter struct {
	Client daze.Dialer
	Netbox *daze.NetBox
}

func (f *Filter) Road(host string) int {
	ips, err := net.LookupIP(host)
	if err != nil {
		return 2
	}
	if f.Netbox.Has(ips[0]) {
		return 0
	}
	return 1
}

func (f *Filter) Dial(network, address string) (io.ReadWriteCloser, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	road := f.Road(host)
	switch road {
	case 0:
		return net.Dial(network, address)
	case 1:
		return f.Client.Dial(network, address)
	}
	conn, err := net.DialTimeout(network, address, time.Second*4)
	if err != nil {
		return f.Client.Dial(network, address)
	}
	return conn, err
}

func NewFilter(dialer daze.Dialer) *Filter {
	netbox := &daze.NetBox{}
	for _, e := range daze.IPv4ReservedIPNet().L {
		netbox.Add(e)
	}
	for _, e := range daze.DarkMainlandIPNet().L {
		netbox.Add(e)
	}
	return &Filter{
		Client: dialer,
		Netbox: netbox,
	}
}
