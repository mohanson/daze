package daze

import (
	"bufio"
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/md5"
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
	"os/user"
	"path"
	"strconv"
	"strings"
	"time"
)

func Link(a, b io.ReadWriteCloser) {
	go func() {
		io.Copy(b, a)
		a.Close()
		b.Close()
	}()
	io.Copy(a, b)
	b.Close()
	a.Close()
}

func IPContains(l []*net.IPNet, ip net.IP) bool {
	for _, entry := range l {
		if entry.Contains(ip) {
			return true
		}
	}
	return false
}

var NativeNet = func() []*net.IPNet {
	priv := []string{
		"0.0.0.0/8",
		"10.0.0.0/8",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"172.16.0.0/12",
		"192.0.0.0/29",
		"192.0.0.170/31",
		"192.0.2.0/24",
		"192.168.0.0/16",
		"198.18.0.0/15",
		"198.51.100.0/24",
		"203.0.113.0/24",
		"240.0.0.0/4",
		"255.255.255.255/32",
	}
	l := []*net.IPNet{}
	for _, n := range priv {
		_, e, _ := net.ParseCIDR(n)
		l = append(l, e)
	}
	return l
}()

func DNSDialer(ctx context.Context, network, address string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "udp", "8.8.8.8:53")
}

var DefaultResolver = &net.Resolver{
	PreferGo: true,
	Dial:     DNSDialer,
}

func LookupIP(host string) ([]net.IP, error) {
	addrs, err := DefaultResolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		return nil, err
	}
	ips := make([]net.IP, len(addrs))
	for i, ia := range addrs {
		ips[i] = ia.IP
	}
	return ips, nil
}

type ReadWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}

func Gravity(conn io.ReadWriteCloser, k []byte) io.ReadWriteCloser {
	cr, _ := rc4.NewCipher(k)
	cw, _ := rc4.NewCipher(k)
	return &ReadWriteCloser{
		Reader: cipher.StreamReader{S: cr, R: conn},
		Writer: cipher.StreamWriter{S: cw, W: conn},
		Closer: conn,
	}
}

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
	connl = Gravity(connl, append(buf[:128], s.Cipher[:]...))

	_, err = io.ReadFull(connl, buf[:128])
	if err != nil {
		return err
	}
	if buf[0] != 0xFF || buf[1] != 0xFF {
		return fmt.Errorf("daze: malformed request: %v", buf[:2])
	}
	d := int64(binary.BigEndian.Uint64(buf[120:128]))
	if math.Abs(float64(time.Now().Unix()-d)) > 120 {
		return fmt.Errorf("daze: expired: %v", time.Unix(d, 0))
	}
	_, err = io.ReadFull(connl, buf[:2+256])
	if err != nil {
		return err
	}
	killer.Stop()
	dst = string(buf[2 : 2+buf[1]])
	log.Println("Connect", dst)
	if buf[0] == 0x03 {
		connr, err = net.DialTimeout("udp", dst, time.Second*8)
	} else {
		connr, err = net.DialTimeout("tcp", dst, time.Second*8)
	}
	if err != nil {
		return err
	}
	defer connr.Close()

	Link(connl, connr)
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

type Dialer interface {
	Dial(network, address string) (io.ReadWriteCloser, error)
}

type Client struct {
	Server string
	Cipher [16]byte
}

func (c *Client) Dial(network, address string) (io.ReadWriteCloser, error) {
	if len(address) > 256 {
		return nil, fmt.Errorf("daze: destination address too long: %s", address)
	}
	var (
		conn io.ReadWriteCloser
		buf  = make([]byte, 1024)
		err  error
	)
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
	conn = Gravity(conn, append(buf[:128], c.Cipher[:]...))

	rand.Read(buf[:386])
	buf[0] = 0xFF
	buf[1] = 0xFF
	binary.BigEndian.PutUint64(buf[120:128], uint64(time.Now().Unix()))
	switch network {
	case "tcp", "tcp4", "tcp6":
		buf[128] = 0x01
	case "udp", "udp4", "udp6":
		buf[128] = 0x03
	}
	buf[129] = uint8(len(address))
	copy(buf[130:], []byte(address))
	_, err = conn.Write(buf[:386])
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
	Dialer Dialer
}

func (l *Locale) ServeProxy(pre []byte, connl io.ReadWriteCloser) error {
	reader := bufio.NewReader(io.MultiReader(bytes.NewReader(pre), connl))
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

func (l *Locale) ServeSocks(pre []byte, connl io.ReadWriteCloser) error {
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

	Link(connl, connr)
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
	if buf[0] == 0x05 {
		err = l.ServeSocks(buf, connl)
	} else {
		err = l.ServeProxy(buf, connl)
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

func NewLocale(listen string, dialer Dialer) *Locale {
	return &Locale{
		Listen: listen,
		Dialer: dialer,
	}
}

type Filter struct {
	Client *Client
	NetBox []*net.IPNet
}

func (f *Filter) Load(r io.Reader) error {
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
		f.NetBox = append(f.NetBox, cidr)
	}
	return nil
}

func (f *Filter) Road(host string) int {
	ips, err := LookupIP(host)
	if err != nil {
		return 2
	}
	if IPContains(NativeNet, ips[0]) || IPContains(f.NetBox, ips[0]) {
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
	log.Println("Connect", road, address)
	switch road {
	case 0:
		return net.DialTimeout(network, address, time.Second*8)
	case 1:
		return f.Client.Dial(network, address)
	}
	conn, err := net.DialTimeout(network, address, time.Second*4)
	if err == nil {
		return conn, nil
	}
	return f.Client.Dial(network, address)
}

func NewFilter(client *Client) (*Filter, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	filePath := path.Join(u.HomeDir, ".daze", "delegated-apnic-latest")
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			os.Mkdir(path.Join(u.HomeDir, ".daze"), 0666)
		} else {
			return nil, err
		}
	}

	var reader io.Reader
	if fileInfo == nil || time.Since(fileInfo.ModTime()) > time.Hour*24*28 {
		r, err := http.Get("http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest")
		if err != nil {
			return nil, err
		}
		defer r.Body.Close()

		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader = io.TeeReader(r.Body, file)
	} else {
		file, err := os.Open(filePath)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader = file
	}
	f := &Filter{
		Client: client,
		NetBox: []*net.IPNet{},
	}
	if err := f.Load(reader); err != nil {
		return nil, err
	}
	return f, nil
}
