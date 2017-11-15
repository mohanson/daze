package daze

import (
	"bufio"
	"bytes"
	"crypto/rc4"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// Link connects 2 conns.
func Link(connl, connr net.Conn) {
	go func() {
		io.Copy(connr, connl)
		connr.Close()
		connl.Close()
	}()
	io.Copy(connl, connr)
	connr.Close()
	connl.Close()
}

// GravityDazeConn is an implementation of the Conn interface for
// TCP network connections.
type GravityDazeConn struct {
	net.Conn
	cipherW *rc4.Cipher
	cipherR *rc4.Cipher
}

// Read reads data into p.
func (g GravityDazeConn) Read(p []byte) (n int, err error) {
	n, err = g.Conn.Read(p)
	g.cipherR.XORKeyStream(p[:n], p[:n])
	return
}

// Write writes the contents of p into the conn.
func (g GravityDazeConn) Write(p []byte) (n int, err error) {
	g.cipherW.XORKeyStream(p, p)
	return g.Conn.Write(p)
}

// GravityDaze returns a new GravityDaze conn. Data read from
// the returned conn will be decoded using rc4 and then returned.
// Data written to the returned conn will be encoded using rc4
// and then written to w.
func GravityDaze(conn net.Conn, k []byte) net.Conn {
	cipherW, _ := rc4.NewCipher(k)
	cipherR, _ := rc4.NewCipher(k)
	return GravityDazeConn{
		Conn:    conn,
		cipherW: cipherW,
		cipherR: cipherR,
	}
}

// A Server responds to daze request.
type Server struct {
	Listen string
}

// Serve handle requests on incoming connections.
func (s *Server) Serve(connl net.Conn) {
	defer connl.Close()

	var (
		buf = make([]byte, 1024)
		err error
	)

	err = func() error {
		connl.SetDeadline(time.Now().Add(time.Second * 10))
		defer connl.SetDeadline(time.Time{})

		_, err = io.ReadFull(connl, buf[:128])
		if err != nil {
			return err
		}

		connl = GravityDaze(connl, buf[:128])

		_, err = io.ReadFull(connl, buf[:128])
		if err != nil {
			return err
		}
		if buf[0] != 0xFF || buf[1] != 0xFF {
			return fmt.Errorf("Malformed request: %v", buf[:2])
		}
		d := int64(binary.BigEndian.Uint64(buf[120:128]))
		if math.Abs(float64(time.Now().Unix()-d)) > 60 {
			return fmt.Errorf("Expired: %v", time.Unix(d, 0))
		}
		return nil
	}()
	if err != nil {
		log.Printf("Serv connect error. %s %v\n", connl.RemoteAddr(), err)
		return
	}

	var (
		dst string
	)

	err = func() error {
		_, err = io.ReadFull(connl, buf[:2+256])
		if err != nil {
			return err
		}
		dst = string(buf[2 : 2+buf[1]])
		return nil
	}()
	if err != nil {
		log.Printf("Recv request error. %s %v\n", connl.RemoteAddr(), err)
		return
	}

	log.Println("Connect", dst)
	connr, err := net.DialTimeout("tcp", dst, time.Second*20)
	if err != nil {
		log.Println(err)
		return
	}
	defer connr.Close()

	Link(connl, connr)
}

// Run listens on the TCP network address addr
// and then calls Serve with Server to handle requests
// on incoming connections.
func (s *Server) Run() {
	ln, err := net.Listen("tcp", s.Listen)
	if err != nil {
		log.Fatalln(err)
	}
	defer ln.Close()
	log.Println("Listen and server on", s.Listen)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go s.Serve(conn)
	}
}

// NewServer returns a Server.
func NewServer(listen string) *Server {
	return &Server{
		Listen: listen,
	}
}

const (
	socks5Version       uint8 = 0x05
	socks5AuthNone      uint8 = 0x00
	socks5AuthNotAccept uint8 = 0xFF
	socks5Connect       uint8 = 0x01
	socks5IP4           uint8 = 0x01
	socks5Domain        uint8 = 0x03
	socks5IP6           uint8 = 0x04
)

var (
	socks5RepSucceeded      = []byte{socks5Version, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	socks5RepGeneralFailure = []byte{socks5Version, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
)

// A Dialer is a means to establish a connection.
type Dialer interface {
	// Dial connects to the given address
	Dial(network, address string) (net.Conn, error)
}

// A Client makes connections to the given address with Server.
type Client struct {
	Server string
}

// Dial connects to the given address via the proxy.
func (c *Client) Dial(network, address string) (net.Conn, error) {
	var (
		conn net.Conn
		buf  = make([]byte, 1024)
		err  error
	)

	conn, err = net.Dial("tcp", c.Server)
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
		return conn, err
	}

	conn = GravityDaze(conn, buf[:128])

	rand.Read(buf[:386])
	buf[0] = 255
	buf[1] = 255
	binary.BigEndian.PutUint64(buf[120:128], uint64(time.Now().Unix()))
	buf[128] = 1
	buf[129] = uint8(len(address))
	copy(buf[130:], []byte(address))
	_, err = conn.Write(buf[:386])
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// NewClient returns a Client.
func NewClient(server string) *Client {
	return &Client{
		Server: server,
	}
}

// A Locale responds to socks5 request and proxy to Server.
type Locale struct {
	Listen string
	Dialer Dialer
}

// ServProxy handle requests on incoming connections using HTTP proxy parser.
func (l *Locale) ServProxy(pre []byte, connl net.Conn) error {
	defer connl.Close()

	reader := bufio.NewReader(io.MultiReader(bytes.NewReader(pre), connl))
	for {
		r, err := http.ReadRequest(reader)
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var port string
		if r.URL.Port() == "" {
			port = "80"
		} else {
			port = r.URL.Port()
		}

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

		Link(connl, connr)
		return nil
	}
}

// ServSocks handle requests on incoming connections using SOCKS5 parser.
func (l *Locale) ServSocks(pre []byte, connl net.Conn) error {
	defer connl.Close()

	var (
		buf     = make([]byte, 1024)
		n       int
		dstType uint8
		dstHost string
		dstPort uint16
		dst     string
		err     error
	)

	err = func() error {
		_, err = io.ReadFull(connl, buf[:1])
		if err != nil {
			return err
		}
		n = int(buf[0])
		_, err = io.ReadFull(connl, buf[:n])
		if err != nil {
			return err
		}
		_, err = connl.Write([]byte{0x05, 0x00})
		if err != nil {
			return err
		}
		return nil
	}()
	if err != nil {
		return fmt.Errorf("Serv connect error: %v", err)
	}

	err = func() error {
		_, err = io.ReadFull(connl, buf[:4])
		if err != nil {
			return err
		}
		dstType = buf[3]
		switch dstType {
		case socks5IP4:
			_, err = io.ReadFull(connl, buf[:4])
			if err != nil {
				return err
			}
			dstHost = net.IP(buf[:4]).String()
		case socks5Domain:
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
		case socks5IP6:
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
		return nil
	}()
	if err != nil {
		return fmt.Errorf("Recv Request error: %v", err)
	}

	var (
		connr net.Conn
	)
	connr, err = l.Dialer.Dial("tcp", dst)
	if err != nil {
		connl.Write(socks5RepGeneralFailure)
		return err
	}
	defer connr.Close()

	connl.Write(socks5RepSucceeded)
	Link(connl, connr)
	return nil
}

// Serve handle requests on incoming connections.
// Note: This func will automatically switch HTTP proxy protocol or SOCKS5 protocol.
func (l *Locale) Serve(connl net.Conn) {
	defer connl.Close()

	var (
		buf = make([]byte, 1)
		err error
	)

	_, err = io.ReadFull(connl, buf)
	if err != nil {
		log.Println(err)
		return
	}

	if buf[0] == 0x05 {
		err = l.ServSocks(buf, connl)
	} else {
		err = l.ServProxy(buf, connl)
	}
	if err != nil {
		log.Printf("Serv %s error: %v", connl.RemoteAddr(), err)
		return
	}
}

// Run listens on the TCP network address addr
// and then calls Serve with Locale to handle requests
// on incoming connections.
func (l *Locale) Run() {
	ln, err := net.Listen("tcp", l.Listen)
	if err != nil {
		log.Fatalln(err)
	}
	defer ln.Close()
	log.Println("Listen and serve on", l.Listen)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go l.Serve(conn)
	}
}

// NewLocale returns a Locale.
func NewLocale(listen string, dialer Dialer) *Locale {
	return &Locale{
		Listen: listen,
		Dialer: dialer,
	}
}

// IPNetList.
type IPNetList []*net.IPNet

// Contains reports whether the network list includes host.
func (n IPNetList) Contains(ip net.IP) bool {
	for _, entry := range n {
		if entry.Contains(ip) {
			return true
		}
	}
	return false
}

const (
	LoopbackNetwork      = "127.0.0.0/8"
	ClassAPrivateNetwork = "10.0.0.0/8"
	ClassBPrivateNetwork = "172.16.0.0/12"
	ClassCPrivateNetwork = "192.168.0.0/16"
)

// NativeNetwork is a network that uses private IP address space.
// Note that localhost has also been joined.
var NativeNetwork = func() IPNetList {
	l := IPNetList{}
	for _, n := range []string{
		LoopbackNetwork,
		ClassAPrivateNetwork,
		ClassBPrivateNetwork,
		ClassCPrivateNetwork,
	} {
		_, e, _ := net.ParseCIDR(n)
		l = append(l, e)
	}
	return l
}()

// CIDRFilter implements Dialer.
type CIDRFilter struct {
	Client     *Client
	CIDRList   IPNetList
	CachedFile string
	URL        string
}

// LoadCIDR from a reader.
func (c *CIDRFilter) LoadCIDRReader(r io.Reader) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "apnic|CN|ipv4") {
			continue
		}
		seps := strings.Split(line, "|")
		sep4, err := strconv.Atoi(seps[4])
		if err != nil {
			return err
		}
		mask := 32 - int(math.Log2(float64(sep4)))
		_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%d", seps[3], mask))
		if err != nil {
			return err
		}
		c.CIDRList = append(c.CIDRList, cidr)
	}
	return nil
}

// LoadCIDR from cached file or URL.
func (c *CIDRFilter) LoadCIDR() error {
	var update bool
	fileinfo, err := os.Stat(c.CachedFile)
	if err != nil {
		if os.IsExist(err) {
			return err
		}
		if err := os.MkdirAll(path.Dir(c.CachedFile), 0666); err != nil {
			return err
		}
		update = true
	} else {
		if time.Since(fileinfo.ModTime()) > time.Hour*24*28 {
			update = true
		}
	}
	if update {
		err := func() error {
			r, err := http.Get(c.URL)
			if err != nil {
				return err
			}
			defer r.Body.Close()

			file, err := os.OpenFile(c.CachedFile, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(file, r.Body)
			return err
		}()
		if err != nil {
			return err
		}
	}
	file, err := os.Open(c.CachedFile)
	if err != nil {
		return err
	}
	defer file.Close()
	return c.LoadCIDRReader(file)
}

// Look reports whether the CIDRList includes host.
func (c *CIDRFilter) Look(host string) int {
	ipList, err := net.LookupIP(host)
	if err != nil {
		return 2
	}
	ip := ipList[0]
	if NativeNetwork.Contains(ip) || c.CIDRList.Contains(ip) {
		return 0
	}
	return 1
}

// Dial.
func (c *CIDRFilter) Dial(network, address string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	road := c.Look(host)
	log.Println("Connect", road, address)
	switch road {
	case 0:
		return net.Dial(network, address)
	case 1:
		return c.Client.Dial(network, address)
	case 2:
		conn, err := net.DialTimeout(network, address, time.Second*4)
		if err == nil {
			return conn, nil
		}
		return c.Client.Dial(network, address)
	}
	return net.Dial(network, address)
}

// NewCIDRFilter returns a CIDRFilter.
func NewCIDRFilter(client *Client) *CIDRFilter {
	return &CIDRFilter{
		Client:     client,
		CIDRList:   IPNetList{},
		CachedFile: "/etc/daze/delegated-apnic-latest",
		URL:        "http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest",
	}
}
