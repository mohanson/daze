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
	"time"

	"github.com/mohanson/acdb"
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

func Resolve(addr string) {
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", addr)
		},
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

func CNIPNet() *NetBox {
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

// NewFilterIP returns a FilterIP.
func NewFilterIP(dialer Dialer) *FilterIP {
	f := &FilterIP{
		Client: dialer,
		Netbox: NetBox{},
	}
	f.Join(IPv4ReservedIPNet())
	f.Join(IPv6ReservedIPNet())
	return f
}

// Filter determines whether the traffic should uses the proxy based on the
// destination's IP address.
type FilterIP struct {
	Client Dialer
	Netbox NetBox
}

// Join adds the IPNet of n.
func (f *FilterIP) Join(n *NetBox) {
	for _, e := range n.L {
		f.Netbox.Add(e)
	}
}

// Dial connects to the address on the named network. If necessary, Filter
// will use f.Client.Dial, else net.Dial instead.
func (f *FilterIP) Dial(network, address string) (io.ReadWriteCloser, error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	ipls, err := net.LookupIP(host)
	if err != nil {
		return net.Dial(network, address)
	}
	if f.Netbox.Has(ipls[0]) {
		return net.Dial(network, address)
	}
	conn, err := f.Client.Dial(network, address)
	if err != nil {
		return net.Dial(network, address)
	}
	return conn, nil
}

// NewFilterAuto returns a FilterAuto. The poor Nier doesn't have enough brain
// capacity, it can only remember 1024 addresses(Because LRU is used to avoid
// memory transition expansion).
func NewFilterAuto(dialer Dialer) *FilterAuto {
	return &FilterAuto{
		Client: dialer,
		Box:    acdb.LRU(1024),
	}
}

// Filter determines whether the traffic should uses the proxy based on the
// destination's IP address or domain. Different from FilterIP, FilterAuto
// is a fuck smart monkey, it first tries to connect to the address using a
// local connection, if it fails, will using the proxy instead. This
// experience will be remembered by this monkey, so next time it sees the same
// address again, Nier(I just gave it the name) will make a decision
// immediately.
type FilterAuto struct {
	Client Dialer
	Box    acdb.Client
}

// Dial connects to the address on the named network. If necessary, Filter
// will use f.Client.Dial, else net.Dial instead.
func (f *FilterAuto) Dial(network, address string) (io.ReadWriteCloser, error) {
	var p bool
	f.Box.Get(address, &p)
	if p {
		return f.Client.Dial(network, address)
	}
	connl, connlErr := net.DialTimeout(network, address, time.Second*2)
	if connlErr == nil {
		return connl, nil
	}
	connr, connrErr := f.Client.Dial(network, address)
	if connrErr == nil {
		f.Box.Set(address, true)
		return connr, nil
	}
	return nil, connrErr
}

type Locale struct {
	Listen string
	Dialer Dialer
}

func (l *Locale) ServeProxy(connl io.ReadWriteCloser) error {
	connlReader := bufio.NewReader(connl)

	for {
		if err := func() error {
			r, err := http.ReadRequest(connlReader)
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
			defer connr.Close()
			connrReader := bufio.NewReader(connr)

			if r.Method == "CONNECT" {
				log.Println("Connect[tunnel]", r.URL.Hostname()+":"+port)
				_, err := connl.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
				if err != nil {
					return err
				}
				Link(connl, connr)
				return nil
			}

			log.Println("Connect[hproxy]", r.URL.Hostname()+":"+port)
			if r.Method == "GET" && r.Header.Get("Upgrade") == "websocket" {
				if err := r.Write(connr); err != nil {
					return err
				}
				Link(connl, connr)
				return nil
			}
			if err := r.Write(connr); err != nil {
				return err
			}
			resp, err := http.ReadResponse(connrReader, r)
			if err != nil {
				return err
			}
			return resp.Write(connl)
		}(); err != nil {
			break
		}
	}
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

	io.ReadFull(connl, buf[:2])
	io.ReadFull(connl, buf[:2])
	dstPort = binary.BigEndian.Uint16(buf[:2])
	io.ReadFull(connl, buf[:4])
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
	log.Println("Connect[socks4]", dst)

	connr, err = l.Dialer.Dial("tcp", dst)
	if err != nil {
		connl.Write([]byte{0x00, 0x5B, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return err
	}
	defer connr.Close()
	connl.Write([]byte{0x00, 0x5A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

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

	io.ReadFull(connl, buf[:2])
	n = int(buf[1])
	io.ReadFull(connl, buf[:n])
	connl.Write([]byte{0x05, 0x00})
	io.ReadFull(connl, buf[:4])
	dstNetwork = buf[1]
	dstCase = buf[3]
	switch dstCase {
	case 0x01:
		io.ReadFull(connl, buf[:4])
		dstHost = net.IP(buf[:4]).String()
	case 0x03:
		io.ReadFull(connl, buf[:1])
		n = int(buf[0])
		io.ReadFull(connl, buf[:n])
		dstHost = string(buf[:n])
	case 0x04:
		io.ReadFull(connl, buf[:16])
		dstHost = net.IP(buf[:16]).String()
	}
	_, err = io.ReadFull(connl, buf[:2])
	if err != nil {
		return err
	}
	dstPort = binary.BigEndian.Uint16(buf[:2])
	dst = dstHost + ":" + strconv.Itoa(int(dstPort))
	log.Println("Connect[socks5]", dst)

	if dstNetwork == 0x03 {
		connr, err = l.Dialer.Dial("udp", dst)
	} else {
		connr, err = l.Dialer.Dial("tcp", dst)
	}
	if err != nil {
		connl.Write([]byte{0x05, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		return err
	}
	defer connr.Close()
	connl.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})

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
		return l.ServeSocks5(connl)
	}
	if buf[0] == 0x04 {
		return l.ServeSocks4(connl)
	}
	return l.ServeProxy(connl)
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
