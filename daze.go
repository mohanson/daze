package daze

import (
	"bufio"
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/rc4"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
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

type ReadWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}

func GravityReader(r io.Reader, k []byte) io.Reader {
	cr, _ := rc4.NewCipher(k)
	return cipher.StreamReader{S: cr, R: r}
}

func GravityWriter(w io.Writer, k []byte) io.Writer {
	cw, _ := rc4.NewCipher(k)
	return cipher.StreamWriter{S: cw, W: w}
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

type Dialer interface {
	Dial(network, address string) (io.ReadWriteCloser, error)
}

type Server interface {
	Serve(connl io.ReadWriteCloser) error
	Run() error
}

type NetBox struct {
	L []*net.IPNet
}

func (n *NetBox) Add(ipNet *net.IPNet) {
	n.L = append(n.L, ipNet)
}

func (n *NetBox) Has(ip net.IP) bool {
	for _, entry := range n.L {
		if entry.Contains(ip) {
			return true
		}
	}
	return false
}

func IPv4ReservedIPNet() *NetBox {
	netBox := &NetBox{}
	for _, entry := range [][2]string{
		[2]string{"00000000", "FF000000"},
		[2]string{"0A000000", "FF000000"},
		[2]string{"7F000000", "FF000000"},
		[2]string{"A9FE0000", "FFFF0000"},
		[2]string{"AC100000", "FFF00000"},
		[2]string{"C0000000", "FFFFFFF8"},
		[2]string{"C00000AA", "FFFFFFFE"},
		[2]string{"C0000200", "FFFFFF00"},
		[2]string{"C0A80000", "FFFF0000"},
		[2]string{"C6120000", "FFFE0000"},
		[2]string{"C6336400", "FFFFFF00"},
		[2]string{"CB007100", "FFFFFF00"},
		[2]string{"F0000000", "F0000000"},
		[2]string{"FFFFFFFF", "FFFFFFFF"},
	} {
		i, _ := hex.DecodeString(entry[0])
		m, _ := hex.DecodeString(entry[1])
		netBox.Add(&net.IPNet{IP: i, Mask: m})
	}
	return netBox
}

func IPv6ReservedIPNet() *NetBox {
	netBox := &NetBox{}
	for _, entry := range [][2]string{
		[2]string{"00000000000000000000000000000000", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"},
		[2]string{"00000000000000000000000000000001", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"},
		[2]string{"00000000000000000000FFFF00000000", "FFFFFFFFFFFFFFFFFFFFFFFF00000000"},
		[2]string{"01000000000000000000000000000000", "FFFFFFFFFFFFFFFF0000000000000000"},
		[2]string{"0064FF9B000000000000000000000000", "FFFFFFFFFFFFFFFFFFFFFFFF00000000"},
		[2]string{"20010000000000000000000000000000", "FFFFFFFF000000000000000000000000"},
		[2]string{"20010010000000000000000000000000", "FFFFFFF0000000000000000000000000"},
		[2]string{"20010020000000000000000000000000", "FFFFFFF0000000000000000000000000"},
		[2]string{"20010DB8000000000000000000000000", "FFFFFFFF000000000000000000000000"},
		[2]string{"20020000000000000000000000000000", "FFFF0000000000000000000000000000"},
		[2]string{"FC000000000000000000000000000000", "FE000000000000000000000000000000"},
		[2]string{"FE800000000000000000000000000000", "FFC00000000000000000000000000000"},
		[2]string{"FF000000000000000000000000000000", "FF000000000000000000000000000000"},
	} {
		i, _ := hex.DecodeString(entry[0])
		m, _ := hex.DecodeString(entry[1])
		netBox.Add(&net.IPNet{IP: i, Mask: m})
	}
	return netBox
}

func DarkMainlandIPNet() *NetBox {
	r, err := http.Get("http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest")
	if err != nil {
		log.Fatalln(err)
	}
	defer r.Body.Close()

	netBox := &NetBox{}
	s := bufio.NewScanner(r.Body)
	for s.Scan() {
		line := s.Text()
		if !strings.HasPrefix(line, "apnic|CN|ipv4") {
			continue
		}
		seps := strings.Split(line, "|")
		sep4, err := strconv.Atoi(seps[4])
		if err != nil {
			log.Fatalln(err)
		}
		mask := 32 - int(math.Log2(float64(sep4)))
		_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%d", seps[3], mask))
		if err != nil {
			log.Fatalln(err)
		}
		netBox.Add(cidr)
	}
	return netBox
}

func SetResolver(addr string) {
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", addr)
		},
	}
}

type Filter struct {
	Client Dialer
	Netbox NetBox
}

func (f *Filter) Dial(network, address string) (io.ReadWriteCloser, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	if f.Netbox.Has(ips[0]) {
		return net.Dial(network, address)
	}
	return f.Client.Dial(network, address)
}

func NewFilter(dialer Dialer) *Filter {
	netbox := NetBox{}
	for _, e := range IPv4ReservedIPNet().L {
		netbox.Add(e)
	}
	for _, e := range DarkMainlandIPNet().L {
		netbox.Add(e)
	}
	return &Filter{
		Client: dialer,
		Netbox: netbox,
	}
}

type Locale struct {
	Listen string
	Dialer Dialer
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

	Link(connl, connr)
	return nil
}

func (l *Locale) ServeSocks4(connl io.ReadWriteCloser) error {
	var (
		buf          = make([]byte, 1024)
		reader       = bufio.NewReader(connl)
		dstHostBytes []byte
		dstHost      string
		dstPort      uint16
		dst          string
		connr        io.ReadWriteCloser
		err          error
	)

	connl = ReadWriteCloser{
		Reader: reader,
		Writer: connl,
		Closer: connl,
	}

	_, err = io.ReadFull(connl, buf[:2])
	if err != nil {
		return err
	}
	_, err = io.ReadFull(connl, buf[:2])
	if err != nil {
		return err
	}
	dstPort = binary.BigEndian.Uint16(buf[:2])
	_, err = io.ReadFull(connl, buf[:4])
	if err != nil {
		return err
	}
	_, err = reader.ReadBytes(0x00)
	if err != nil {
		return err
	}
	if bytes.Equal(buf[:3], []byte{0x00, 0x00, 0x00}) && buf[3] != 0x00 {
		dstHostBytes, err = reader.ReadBytes(0x00)
		if err != nil {
			return err
		}
		dstHost = string(dstHostBytes[:len(dstHostBytes)-1])
	} else {
		dstHost = net.IP(buf[:4]).String()
	}
	dst = dstHost + ":" + strconv.Itoa(int(dstPort))
	log.Println("Connect", dst)

	connr, err = l.Dialer.Dial("tcp", dst)
	if err != nil {
		connl.Write([]byte{0x00, 0x5B, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return err
	}
	connl.Write([]byte{0x00, 0x5A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	defer connr.Close()

	Link(connl, connr)
	return nil
}

func (l *Locale) ServeSocks5(connl io.ReadWriteCloser) error {
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
	connl = ReadWriteCloser{
		Reader: io.MultiReader(bytes.NewReader(buf), connl),
		Writer: connl,
		Closer: connl,
	}
	if buf[0] == 0x05 {
		err = l.ServeSocks5(connl)
	} else if buf[0] == 0x04 {
		err = l.ServeSocks4(connl)
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

func NewLocale(listen string, dialer Dialer) *Locale {
	return &Locale{
		Listen: listen,
		Dialer: dialer,
	}
}
