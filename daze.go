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
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/godump/acdb"
	"github.com/godump/aget"
	"github.com/godump/ddir"
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
	Dial(network string, address string) (io.ReadWriteCloser, error)
}

// IPv4ReservedIPNet returns reserved IPv4 addresses.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/Reserved_IP_addresses
func IPv4ReservedIPNet() []*net.IPNet {
	r := []*net.IPNet{}
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
		r = append(r, &net.IPNet{IP: i, Mask: m})
	}
	return r
}

// IPv6ReservedIPNet returns reserved IPv6 addresses.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/Reserved_IP_addresses
func IPv6ReservedIPNet() []*net.IPNet {
	r := []*net.IPNet{}
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
		r = append(r, &net.IPNet{IP: i, Mask: m})
	}
	return r
}

// CNIPNet returns full ipv4/6 CIDR in CN.
func CNIPNet() []*net.IPNet {
	furl := "http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest"
	name := ddir.Join("delegated-apnic-latest")
	f, err := aget.OpenEx(furl, name, time.Hour*24*64)
	if err != nil {
		log.Panicln(err)
	}
	defer f.Close()
	r := []*net.IPNet{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "apnic|CN|ipv4"):
			seps := strings.Split(line, "|")
			sep4, err := strconv.Atoi(seps[4])
			if err != nil {
				log.Panicln(err)
			}
			mask := 32 - int(math.Log2(float64(sep4)))
			_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%d", seps[3], mask))
			if err != nil {
				log.Panicln(err)
			}
			r = append(r, cidr)
		case strings.HasPrefix(line, "apnic|CN|ipv6"):
			seps := strings.Split(line, "|")
			sep4 := seps[4]
			_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%s", seps[3], sep4))
			if err != nil {
				log.Panicln(err)
			}
			r = append(r, cidr)
		}
	}
	return r
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
func (l *Locale) ServeProxy(conn io.ReadWriteCloser) error {
	reader := bufio.NewReader(conn)

	for {
		if err := func() error {
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

			serv, err := l.Dialer.Dial("tcp", r.URL.Hostname()+":"+port)
			if err != nil {
				return err
			}
			defer serv.Close()
			servReader := bufio.NewReader(serv)

			if r.Method == "CONNECT" {
				log.Println("Connect[tunnel]", r.URL.Hostname()+":"+port)
				_, err := conn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
				if err != nil {
					return err
				}
				Link(conn, serv)
				return nil
			}

			log.Println("Connect[hproxy]", r.URL.Hostname()+":"+port)
			if r.Method == "GET" && r.Header.Get("Upgrade") == "websocket" {
				if err := r.Write(serv); err != nil {
					return err
				}
				Link(conn, serv)
				return nil
			}
			if err := r.Write(serv); err != nil {
				return err
			}
			resp, err := http.ReadResponse(servReader, r)
			if err != nil {
				return err
			}
			return resp.Write(conn)
		}(); err != nil {
			break
		}
	}
	return nil
}

// Serve traffic in SOCKS4/SOCKS4a format.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/SOCKS
//   See http://ftp.icm.edu.pl/packages/socks/socks4/SOCKS4.protocol
func (l *Locale) ServeSocks4(conn io.ReadWriteCloser) error {
	var (
		reader    = bufio.NewReader(conn)
		fCode     uint8
		fDstPort  = make([]byte, 2)
		fDstIP    = make([]byte, 4)
		fHostName []byte
		dstHost   string
		dstPort   uint16
		dst       string
		serv      io.ReadWriteCloser
		err       error
	)
	conn = ReadWriteCloser{
		Reader: reader,
		Writer: conn,
		Closer: conn,
	}
	reader.Discard(1)
	fCode, _ = reader.ReadByte()
	io.ReadFull(reader, fDstPort)
	dstPort = binary.BigEndian.Uint16(fDstPort)
	io.ReadFull(reader, fDstIP)
	_, err = reader.ReadBytes(0x00)
	if err != nil {
		return err
	}
	if bytes.Equal(fDstIP[:3], []byte{0x00, 0x00, 0x00}) && fDstIP[3] != 0x00 {
		fHostName, err = reader.ReadBytes(0x00)
		if err != nil {
			return err
		}
		fHostName = fHostName[:len(fHostName)-1]
		dstHost = string(fHostName)
	} else {
		dstHost = net.IP(fDstIP).String()
	}
	dst = dstHost + ":" + strconv.Itoa(int(dstPort))
	log.Println("Connect[socks4]", dst)
	switch fCode {
	case 0x01:
		serv, err = l.Dialer.Dial("tcp", dst)
		if err != nil {
			conn.Write([]byte{0x00, 0x5B, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
			return err
		}
		defer serv.Close()
		conn.Write([]byte{0x00, 0x5A, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		Link(conn, serv)
		return nil
	case 0x02:
	}
	return nil
}

// Serve traffic in SOCKS5 format.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/SOCKS
//   See https://tools.ietf.org/html/rfc1928
func (l *Locale) ServeSocks5(conn io.ReadWriteCloser) error {
	var (
		reader   = bufio.NewReader(conn)
		fN       uint8
		fCmd     uint8
		fAT      uint8
		fDstAddr []byte
		fDstPort = make([]byte, 2)
		dstHost  string
		dstPort  uint16
		dst      string
		serv     io.ReadWriteCloser
		err      error
	)
	conn = ReadWriteCloser{
		Reader: reader,
		Writer: conn,
		Closer: conn,
	}
	reader.Discard(1)
	fN, _ = reader.ReadByte()
	reader.Discard(int(fN))
	conn.Write([]byte{0x05, 0x00})
	reader.Discard(1)
	fCmd, _ = reader.ReadByte()
	reader.Discard(1)
	fAT, _ = reader.ReadByte()
	switch fAT {
	case 0x01:
		fDstAddr = make([]byte, 4)
		io.ReadFull(reader, fDstAddr)
		dstHost = net.IP(fDstAddr).String()
	case 0x03:
		fN, _ = reader.ReadByte()
		fDstAddr = make([]byte, int(fN))
		io.ReadFull(reader, fDstAddr)
		dstHost = string(fDstAddr)
	case 0x04:
		fDstAddr = make([]byte, 16)
		io.ReadFull(reader, fDstAddr)
		dstHost = net.IP(fDstAddr).String()
	}
	if _, err = io.ReadFull(conn, fDstPort); err != nil {
		return err
	}
	dstPort = binary.BigEndian.Uint16(fDstPort)
	dst = dstHost + ":" + strconv.Itoa(int(dstPort))
	log.Println("Connect[socks5]", dst)
	switch fCmd {
	case 0x01:
		serv, err = l.Dialer.Dial("tcp", dst)
		if err != nil {
			conn.Write([]byte{0x05, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
			return err
		}
		defer serv.Close()
		conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		Link(conn, serv)
		return nil
	case 0x02:
	case 0x03:
	}
	return nil
}

// We should be very clear about what it does. It judges the traffic type and
// processes it with a different handler(ServeProxy/ServeSocks4/ServeSocks5).
func (l *Locale) Serve(conn io.ReadWriteCloser) error {
	var (
		buf = make([]byte, 1)
		err error
	)
	_, err = io.ReadFull(conn, buf)
	if err != nil {
		return err
	}
	conn = ReadWriteCloser{
		Reader: io.MultiReader(bytes.NewReader(buf), conn),
		Writer: conn,
		Closer: conn,
	}
	if buf[0] == 0x05 {
		return l.ServeSocks5(conn)
	}
	if buf[0] == 0x04 {
		return l.ServeSocks4(conn)
	}
	return l.ServeProxy(conn)
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
		go func() {
			defer conn.Close()
			if err := l.Serve(conn); err != nil {
				log.Println(err)
			}
		}()
	}
}

// NewLocale returns a Locale.
func NewLocale(listen string, dialer Dialer) *Locale {
	return &Locale{
		Listen: listen,
		Dialer: dialer,
	}
}

// Direct is the default dialer for connecting to an address.
type Direct struct {
}

func (d *Direct) Dial(network string, address string) (io.ReadWriteCloser, error) {
	return net.Dial(network, address)
}

// A RoadMode represents a host's road mode.
type RoadMode int

const (
	MLocale RoadMode = iota
	MRemote
	MFucked
	MPuzzle
)

// RULE file aims to be a minimal configuration file format that's easy to
// read due to obvious semantics.
// There are two parts per line on RULE file: mode and glob. mode are on the
// left of the space sign and glob are on the right. mode is an char and
// describes whether the host should go proxy, glob supported glob-style
// patterns:
//   h?llo matches hello, hallo and hxllo
//   h*llo matches hllo and heeeello
//   h[ae]llo matches hello and hallo, but not hillo
//   h[^e]llo matches hallo, hbllo, ... but not hello
//   h[a-b]llo matches hallo and hbllo
//
// This is a RULE document:
//   F a.com b.com
//   L a.com
//   R b.com
//   B c.com
//
// F(orward) means using b.com instead of a.com
// L(ocale)  means using locale network
// R(emote)  means using remote network
// B(anned)  means block it
type Rulels struct {
	Host map[string]string
	Rule map[string]RoadMode
}

func (r *Rulels) Road(host string) RoadMode {
	for p, i := range r.Rule {
		b, err := filepath.Match(p, host)
		if err != nil {
			log.Panicln(err)
		}
		if !b {
			continue
		}
		return i
	}
	return MPuzzle
}

// Load a RULE file.
func (r *Rulels) Load(name string) error {
	f, err := aget.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		seps := strings.Split(line, " ")
		if len(seps) < 2 {
			continue
		}
		switch seps[0] {
		case "#":
		case "F":
			r.Host[seps[1]] = seps[2]
		case "L":
			for _, e := range seps[1:] {
				r.Rule[e] = MLocale
			}
		case "R":
			for _, e := range seps[1:] {
				r.Rule[e] = MRemote
			}
		case "B":
			for _, e := range seps[1:] {
				r.Rule[e] = MFucked
			}
		}
	}
	return scanner.Err()
}

// NewRoaderRule returns a new RoaderRule.
func NewRulels() *Rulels {
	return &Rulels{
		Host: map[string]string{},
		Rule: map[string]RoadMode{},
	}
}

type Squire struct {
	Dialer Dialer
	Direct Dialer
	Memory acdb.Client
	Rulels *Rulels
	IPNets []*net.IPNet
}

func (s *Squire) Dial(network string, address string) (io.ReadWriteCloser, error) {
	var (
		host string
		port string
		mode RoadMode
		ips  []net.IP
		err  error
	)
	host, port, err = net.SplitHostPort(address)

	if a, ok := s.Rulels.Host[host]; ok {
		host = a
		address = host + ":" + port
	}

	err = s.Memory.Get(host, &mode)
	if err == nil {
		switch mode {
		case MLocale:
			return s.Direct.Dial(network, address)
		case MRemote:
			return s.Dialer.Dial(network, address)
		}
		log.Panicln("")
	}
	if err != acdb.ErrNotExist {
		return nil, err
	}

	switch s.Rulels.Road(host) {
	case MLocale:
		s.Memory.Set(host, MLocale)
		return s.Direct.Dial(network, address)
	case MRemote:
		s.Memory.Set(host, MRemote)
		return s.Dialer.Dial(network, address)
	case MFucked:
		return nil, fmt.Errorf("daze: %s has been blocked", host)
	case MPuzzle:
	}

	ips, err = net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	if func() int {
		for _, e := range s.IPNets {
			if e.Contains(ips[0]) {
				return 1
			}
		}
		return 0
	}() == 1 {
		s.Memory.Set(host, MLocale)
		return s.Direct.Dial(network, address)
	} else {
		s.Memory.Set(host, MRemote)
		return s.Dialer.Dial(network, address)
	}
}

// NewSquire.
func NewSquire(dialer Dialer) *Squire {
	return &Squire{
		Dialer: dialer,
		Direct: &Direct{},
		Memory: acdb.Lru(1024),
		Rulels: NewRulels(),
	}
}
