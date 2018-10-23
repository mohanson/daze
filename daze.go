package daze

import (
	"bufio"
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/rc4"
	"encoding/binary"
	"encoding/hex"
	"errors"
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

// Link copies from src to dst and dst to src until either EOF is reached.
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

// ReadWriteCloser is the interface that groups the basic Read, Write and
// Close methods.
type ReadWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}

// GravityReader wraps an io.Reader with RC4 crypto.
func GravityReader(r io.Reader, k []byte) io.Reader {
	cr, _ := rc4.NewCipher(k)
	return cipher.StreamReader{S: cr, R: r}
}

// GravityWriter wraps an io.Writer with RC4 crypto.
func GravityWriter(w io.Writer, k []byte) io.Writer {
	cw, _ := rc4.NewCipher(k)
	return cipher.StreamWriter{S: cw, W: w}
}

// Double gravity, double happiness.
func Gravity(conn io.ReadWriteCloser, k []byte) io.ReadWriteCloser {
	cr, _ := rc4.NewCipher(k)
	cw, _ := rc4.NewCipher(k)
	return &ReadWriteCloser{
		Reader: cipher.StreamReader{S: cr, R: conn},
		Writer: cipher.StreamWriter{S: cw, W: conn},
		Closer: conn,
	}
}

// Resolve modifies the net.DefaultResolver(which is the resolver used by the
// package-level Lookup functions and by Dialers without a specified Resolver).
//
// Examples:
//   Resolve("8.8.8.8:53")
//   Resolve("114.114.114.114:53")
func Resolve(addr string) {
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", addr)
		},
	}
}

// Dialer contains options for connecting to an address.
type Dialer interface {
	Dial(network, address string) (io.ReadWriteCloser, error)
}

// Server is the main process of daze. In most cases, it is usually deployed
// as a daemon on a linux machine.
//
// Different protocols implement different Servers. In the current version,
// daze implements few protocols. the source code is located:
//   ./protocol/ashe
//   ./protocol/asheshadow
//
// You can easily implement your own protocals to fight against the watching
// of the big brother.
type Server interface {
	Serve(connl io.ReadWriteCloser) error
	Run() error
}

// NetBox is the collection of *net.IPNet. It just provides some easy ways.
type NetBox struct {
	L []*net.IPNet
}

// Add a new *net.IPNet into NetBox.
func (n *NetBox) Add(ipNet *net.IPNet) {
	n.L = append(n.L, ipNet)
}

// Mrg is short for "Merge".
func (n *NetBox) Mrg(box *NetBox) {
	for _, e := range box.L {
		n.Add(e)
	}
}

// Whether ip is in the collection.
func (n *NetBox) Has(ip net.IP) bool {
	for _, entry := range n.L {
		if entry.Contains(ip) {
			return true
		}
	}
	return false
}

// IPv4ReservedIPNet returns reserved IPv4 addresses.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/Reserved_IP_addresses
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

// IPv6ReservedIPNet returns reserved IPv6 addresses.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/Reserved_IP_addresses
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

// CNIPNet returns full ipv4 CIDR in CN.
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

const (
	RoadRemote = 0x00
	RoadLocale = 0x01
	MoldNier   = 0x00
	MoldIP     = 0x01
)

// NewFilter returns a Filter. The poor Nier doesn't have enough brain
// capacity, it can only remember 1024 addresses(Because LRU is used to avoid
// memory transition expansion).
func NewFilter(dialer Dialer) *Filter {
	return &Filter{
		Client: dialer,
		NetBox: NetBox{},
		Namedb: acdb.LRU(1024),
		Mold:   MoldIP,
	}
}

// Filter determines whether the traffic should uses the proxy based on the
// destination's IP address or domain.
// There are two criteria to define a Filter:
//   - MoldNier
//   - MoldIP
//
// MoldNier is a fuck smart monkey, it first tries to connect to the address
// by local connection, if it fails, then use the proxy. This experience will
// be remembered by this monkey, so next time Nier will make a decision
// immediately when it sees the same address again.
//
// MoldIP force switching based on IP address.
type Filter struct {
	Client Dialer
	NetBox NetBox
	Namedb acdb.Client
	Mold   int
}

// Dial connects to the address on the named network. If necessary, Filter
// will use f.Client.Dial, else net.Dial instead.
func (f *Filter) Dial(network, address string) (io.ReadWriteCloser, error) {
	var (
		host   string
		choose int
		connl  io.ReadWriteCloser
		connr  io.ReadWriteCloser
		err    error
	)
	host, _, err = net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	err = f.Namedb.Get(host, &choose)
	if err == nil {
		switch choose {
		case RoadRemote:
			return f.Client.Dial(network, address)
		case RoadLocale:
			return net.Dial(network, address)
		}
		log.Fatalln("")
	}
	if err != acdb.ErrNotExist {
		return nil, err
	}
	switch f.Mold {
	case MoldNier:
		connl, err = net.DialTimeout(network, address, time.Second*4)
		if err == nil {
			f.Namedb.SetNone(host, RoadLocale)
			return connl, nil
		}
		connr, err = f.Client.Dial(network, address)
		if err == nil {
			f.Namedb.SetNone(host, RoadRemote)
			return connr, nil
		}
		return nil, err
	case MoldIP:
		ipls, err := net.LookupIP(host)
		if err != nil {
			return nil, err
		}
		if f.NetBox.Has(ipls[0]) {
			f.Namedb.SetNone(host, RoadLocale)
			return net.Dial(network, address)
		}
		f.Namedb.SetNone(host, RoadRemote)
		return f.Client.Dial(network, address)
	}
	return nil, errors.New("daze: unknown mold")
}

// Locale is the main process of daze. In most cases, it is usually deployed
// as a daemon on a local machine.
type Locale struct {
	Listen string
	Dialer Dialer
}

// Serve traffic in HTTP Proxy/Tunnel format.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/Proxy_server
//   See https://en.wikipedia.org/wiki/HTTP_tunnel
//
// Warning: The performance of HTTP Proxy is very poor, unless you have a good
// reason, please use ServeSocks4 or ServeSocks5 instead. Why the poor
// performance is that I did not implement http persistent connection(a
// well-known name is KeepAlive) because It will trigger some bugs on Firefox.
// Firefox always sends traffic from different sites to the one persistent
// connection. I have been debugging for a long time.
// Fuck.
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

// Serve traffic in SOCKS4/SOCKS4a format.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/SOCKS.
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

// Serve traffic in SOCKS5 format.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/SOCKS.
//   See https://tools.ietf.org/html/rfc1928
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

// We should be very clear about what it does. It judges the traffic type and
// processes it with a different handler(ServeProxy/ServeSocks4/ServeSocks5).
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

// Run.
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

// NewLocale returns a Locale.
func NewLocale(listen string, dialer Dialer) *Locale {
	return &Locale{
		Listen: listen,
		Dialer: dialer,
	}
}
