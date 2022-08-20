package daze

import (
	"bufio"
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rc4"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math"
	"math/bits"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/godump/doa"
	"github.com/godump/lru"
)

// ============================================================================
//               ___           ___           ___           ___
//              /\  \         /\  \         /\  \         /\  \
//             /::\  \       /::\  \        \:\  \       /::\  \
//            /:/\:\  \     /:/\:\  \        \:\  \     /:/\:\  \
//           /:/  \:\__\   /::\~\:\  \        \:\  \   /::\~\:\  \
//          /:/__/ \:|__| /:/\:\ \:\__\ _______\:\__\ /:/\:\ \:\__\
//          \:\  \ /:/  / \/__\:\/:/  / \::::::::/__/ \:\~\:\ \/__/
//           \:\  /:/  /       \::/  /   \:\~~\~~      \:\ \:\__\
//            \:\/:/  /        /:/  /     \:\  \        \:\ \/__/
//             \::/__/        /:/  /       \:\__\        \:\__\
//              ~~            \/__/         \/__/         \/__/
// ============================================================================

// Conf is acting as package level configuration.
var Conf = struct {
	Dialer      net.Dialer
	Random      *rand.Rand
	RouterCache int
}{
	Dialer: net.Dialer{
		Resolver: net.DefaultResolver,
		Timeout:  time.Second * 8,
	},
	Random:      rand.New(rand.NewSource(time.Now().Unix())),
	RouterCache: 128,
}

// Resolver returns a new Resolver used by the package-level Lookup functions and by Dialers without a specified
// Resolver.
//
// Examples:
//   Resolver("8.8.8.8:53")
//   Resolver("114.114.114.114:53")
func Resolver(addr string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: Conf.Dialer.Timeout,
			}
			return d.DialContext(ctx, "udp", addr)
		},
	}
}

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

// ReadWriteCloser is the interface that groups the basic Read, Write and Close methods.
type ReadWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}

// Context carries infomations for a tcp connection.
type Context struct {
	Cid uint32
}

// Dialer abstracts the way to establish network connections.
type Dialer interface {
	Dial(ctx *Context, network string, address string) (io.ReadWriteCloser, error)
}

// Direct is the default dialer for connecting to an address.
type Direct struct{}

// Dial implements daze.Dialer.
func (d *Direct) Dial(ctx *Context, network string, address string) (io.ReadWriteCloser, error) {
	return Conf.Dialer.Dial(network, address)
}

// Locale is the main process of daze. In most cases, it is usually deployed as a daemon on a local machine.
type Locale struct {
	Listen string
	Dialer Dialer
	Closer io.Closer
}

// ServeProxy serves traffic in HTTP Proxy/Tunnel format.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/Proxy_server
//   See https://en.wikipedia.org/wiki/HTTP_tunnel
//   See https://www.infoq.com/articles/Web-Sockets-Proxy-Servers/
func (l *Locale) ServeProxy(ctx *Context, app io.ReadWriteCloser) error {
	appReader := bufio.NewReader(app)
	app = ReadWriteCloser{
		Reader: appReader,
		Writer: app,
		Closer: app,
	}
	var err error
	for {
		err = func() error {
			r, err := http.ReadRequest(appReader)
			if err != nil {
				return err
			}

			var port string
			if r.URL.Port() == "" {
				port = "80"
			} else {
				port = r.URL.Port()
			}

			if r.Method == "CONNECT" {
				log.Printf("%08x  proto format=tunnel", ctx.Cid)
			} else {
				log.Printf("%08x  proto format=hproxy", ctx.Cid)
			}

			srv, err := l.Dialer.Dial(ctx, "tcp", r.URL.Hostname()+":"+port)
			if err != nil {
				return err
			}
			defer srv.Close()

			if r.Method == "CONNECT" {
				_, err := app.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
				if err != nil {
					return err
				}
				Link(app, srv)
				return io.EOF
			}
			if r.Method == "GET" && r.Header.Get("Upgrade") == "websocket" {
				if err := r.Write(srv); err != nil {
					return err
				}
				Link(app, srv)
				return io.EOF
			}

			srvReader := bufio.NewReader(srv)
			if err := r.Write(srv); err != nil {
				return err
			}
			s, err := http.ReadResponse(srvReader, r)
			if err != nil {
				return err
			}
			return s.Write(app)
		}()
		if err != nil {
			break
		}
	}
	// It makes no sense to report a EOF error.
	if err == io.EOF {
		return nil
	}
	return err
}

// ServeSocks4 serves traffic in SOCKS4/SOCKS4a format.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/SOCKS
//   See http://ftp.icm.edu.pl/packages/socks/socks4/SOCKS4.protocol
func (l *Locale) ServeSocks4(ctx *Context, app io.ReadWriteCloser) error {
	appReader := bufio.NewReader(app)
	app = ReadWriteCloser{
		Reader: appReader,
		Writer: app,
		Closer: app,
	}
	var (
		fCode     uint8
		fDstPort  = make([]byte, 2)
		fDstIP    = make([]byte, 4)
		fHostName []byte
		dstHost   string
		dstPort   uint16
		dst       string
		srv       io.ReadWriteCloser
		err       error
	)
	appReader.Discard(1)
	fCode, _ = appReader.ReadByte()
	io.ReadFull(appReader, fDstPort)
	dstPort = binary.BigEndian.Uint16(fDstPort)
	io.ReadFull(appReader, fDstIP)
	_, err = appReader.ReadBytes(0x00)
	if err != nil {
		return err
	}
	if bytes.Equal(fDstIP[:3], []byte{0x00, 0x00, 0x00}) && fDstIP[3] != 0x00 {
		fHostName, err = appReader.ReadBytes(0x00)
		if err != nil {
			return err
		}
		fHostName = fHostName[:len(fHostName)-1]
		dstHost = string(fHostName)
	} else {
		dstHost = net.IP(fDstIP).String()
	}
	dst = dstHost + ":" + strconv.Itoa(int(dstPort))
	log.Printf("%08x  proto format=socks4", ctx.Cid)
	switch fCode {
	case 0x01:
		srv, err = l.Dialer.Dial(ctx, "tcp", dst)
		if err != nil {
			app.Write([]byte{0x00, 0x5b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		} else {
			defer srv.Close()
			app.Write([]byte{0x00, 0x5a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
			Link(app, srv)
		}
		return err
	case 0x02:
		panic("unreachable")
	}
	return nil
}

// ServeSocks5 serves traffic in SOCKS5 format.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/SOCKS
//   See https://tools.ietf.org/html/rfc1928
func (l *Locale) ServeSocks5(ctx *Context, app io.ReadWriteCloser) error {
	appReader := bufio.NewReader(app)
	app = ReadWriteCloser{
		Reader: appReader,
		Writer: app,
		Closer: app,
	}
	var (
		fN       uint8
		fCmd     uint8
		fAT      uint8
		fDstAddr []byte
		fDstPort = make([]byte, 2)
		dstHost  string
		dstPort  uint16
		dst      string
		err      error
	)
	appReader.Discard(1)
	fN, _ = appReader.ReadByte()
	appReader.Discard(int(fN))
	app.Write([]byte{0x05, 0x00})
	appReader.Discard(1)
	fCmd, _ = appReader.ReadByte()
	appReader.Discard(1)
	fAT, _ = appReader.ReadByte()
	switch fAT {
	case 0x01:
		fDstAddr = make([]byte, 4)
		io.ReadFull(appReader, fDstAddr)
		dstHost = net.IP(fDstAddr).String()
	case 0x03:
		fN, _ = appReader.ReadByte()
		fDstAddr = make([]byte, int(fN))
		io.ReadFull(appReader, fDstAddr)
		dstHost = string(fDstAddr)
	case 0x04:
		fDstAddr = make([]byte, 16)
		io.ReadFull(appReader, fDstAddr)
		dstHost = net.IP(fDstAddr).String()
	}
	_, err = io.ReadFull(app, fDstPort)
	if err != nil {
		return err
	}
	dstPort = binary.BigEndian.Uint16(fDstPort)
	dst = dstHost + ":" + strconv.Itoa(int(dstPort))
	switch fCmd {
	case 0x01:
		return l.ServeSocks5TCP(ctx, app, dst)
	case 0x02:
		panic("unreachable")
	case 0x03:
		return l.ServeSocks5UDP(ctx, app)
	}
	return nil
}

// ServeSocks5TCP serves socks5 TCP protocol.
func (l *Locale) ServeSocks5TCP(ctx *Context, app io.ReadWriteCloser, dst string) error {
	log.Printf("%08x  proto format=socks5", ctx.Cid)
	srv, err := l.Dialer.Dial(ctx, "tcp", dst)
	if err != nil {
		app.Write([]byte{0x05, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	} else {
		defer srv.Close()
		app.Write([]byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
		Link(app, srv)
	}
	return err
}

// ServeSocks5UDP serves socks5 UDP protocol.
func (l *Locale) ServeSocks5UDP(ctx *Context, app io.ReadWriteCloser) error {
	var (
		bndAddr     *net.UDPAddr
		bndPort     uint16
		bnd         *net.UDPConn
		appAddr     *net.UDPAddr
		appSize     int
		appHeadSize int
		appHead     []byte
		dstHost     string
		dstPort     uint16
		dst         string
		srv         io.ReadWriteCloser
		b           bool
		cpl         = map[string]io.ReadWriteCloser{}
		buf         = make([]byte, 2048)
		err         error
	)
	bndAddr = doa.Try(net.ResolveUDPAddr("udp", "127.0.0.1:0"))
	bnd = doa.Try(net.ListenUDP("udp", bndAddr))
	defer bnd.Close()
	bndPort = uint16(bnd.LocalAddr().(*net.UDPAddr).Port)
	copy(buf, []byte{0x05, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	binary.BigEndian.PutUint16(buf[8:10], bndPort)
	_, err = app.Write(buf[:10])
	if err != nil {
		return err
	}

	// https://datatracker.ietf.org/doc/html/rfc1928, Page 7, UDP ASSOCIATE:
	// A UDP association terminates when the TCP connection that the UDP ASSOCIATE request arrived on terminates.
	go func() {
		io.Copy(ioutil.Discard, app)
		bnd.Close()
	}()

	for {
		appSize, appAddr, err = bnd.ReadFromUDP(buf)
		if err != nil {
			break
		}
		// 	+----+------+------+----------+----------+----------+
		// 	|RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
		// 	+----+------+------+----------+----------+----------+
		// 	| 2  |  1   |  1   | Variable |    2     | Variable |
		// 	+----+------+------+----------+----------+----------+
		//    The fields in the UDP request header are:
		// 		*  RSV  Reserved X'0000'
		// 		*  FRAG    Current fragment number
		// 		*  ATYP    address type of following addresses:
		// 		   *  IP V4 address: X'01'
		// 		   *  DOMAINNAME: X'03'
		// 		   *  IP V6 address: X'04'
		// 		*  DST.ADDR       desired destination address
		// 		*  DST.PORT       desired destination port
		// 		*  DATA     user data
		doa.Doa(buf[0] == 0x00)
		doa.Doa(buf[1] == 0x00)
		// Implementation of fragmentation is optional; an implementation that does not support fragmentation MUST drop
		// any datagram whose FRAG field is other than X'00'.
		doa.Doa(buf[2] == 0x00)
		switch buf[3] {
		case 0x01:
			appHeadSize = 10
		case 0x03:
			appHeadSize = int(buf[4]) + 7
		case 0x04:
			appHeadSize = 22
		}

		appHead = make([]byte, appHeadSize)
		copy(appHead, buf[0:appHeadSize])

		switch appHead[3] {
		case 0x01:
			dstHost = net.IP(appHead[4:8]).String()
			dstPort = binary.BigEndian.Uint16(appHead[8:10])
		case 0x03:
			l := appHead[4]
			dstHost = string(appHead[5 : 5+l])
			dstPort = binary.BigEndian.Uint16(appHead[5+l : 7+l])
		case 0x04:
			dstHost = net.IP(appHead[4:20]).String()
			dstPort = binary.BigEndian.Uint16(appHead[20:22])
		}
		dst = dstHost + ":" + strconv.Itoa(int(dstPort))

		srv, b = cpl[dst]
		if b {
			goto send
		} else {
			goto init
		}
	init:
		log.Printf("%08x  proto format=socks5", ctx.Cid)
		srv, err = l.Dialer.Dial(ctx, "udp", dst)
		if err != nil {
			log.Printf("%08x  error %s", ctx.Cid, err)
			continue
		}
		cpl[dst] = srv
		go func(srv io.ReadWriteCloser, appHead []byte, appAddr *net.UDPAddr) error {
			var (
				buf = make([]byte, 2048)
				l   = len(appHead)
				n   int
				err error
			)
			copy(buf, appHead)
			for {
				n, err = srv.Read(buf[l:])
				if err != nil {
					break
				}
				_, err = bnd.WriteToUDP(buf[:l+n], appAddr)
				if err != nil {
					break
				}
			}
			return err
		}(srv, appHead, appAddr)
	send:
		_, err = srv.Write(buf[appHeadSize:appSize])
		if err != nil {
			log.Printf("%08x  error %s", ctx.Cid, err)
			continue
		}
	}
	for _, e := range cpl {
		e.Close()
	}
	return nil
}

// Serve serves incoming connections and handle it with a different handler(ServeProxy/ServeSocks4/ServeSocks5).
func (l *Locale) Serve(ctx *Context, app io.ReadWriteCloser) error {
	var (
		buf = make([]byte, 1)
		err error
	)
	_, err = io.ReadFull(app, buf)
	if err != nil {
		// There are some clients that will establish a link in advance without sending any messages so that they can
		// immediately get the connected conn when they really need it. When they leave, it makes no sense to report a
		// EOF error.
		if err == io.EOF {
			return nil
		}
		return err
	}
	app = ReadWriteCloser{
		Reader: io.MultiReader(bytes.NewReader(buf), app),
		Writer: app,
		Closer: app,
	}
	if buf[0] == 0x05 {
		return l.ServeSocks5(ctx, app)
	}
	if buf[0] == 0x04 {
		return l.ServeSocks4(ctx, app)
	}
	return l.ServeProxy(ctx, app)
}

// Close listener.
func (l *Locale) Close() error {
	if l.Closer != nil {
		return l.Closer.Close()
	}
	return nil
}

// Run it.
func (l *Locale) Run() error {
	s, err := net.Listen("tcp", l.Listen)
	if err != nil {
		return err
	}
	l.Closer = s
	log.Println("listen and serve on", l.Listen)

	go func() {
		idx := uint32(math.MaxUint32)
		for {
			cli, err := s.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Println(err)
				}
				break
			}
			idx++
			ctx := &Context{idx}
			log.Printf("%08x accept remote=%s", ctx.Cid, cli.RemoteAddr())
			go func(ctx *Context, cli net.Conn) {
				defer cli.Close()
				if err := l.Serve(ctx, cli); err != nil {
					log.Printf("%08x  error %s", ctx.Cid, err)
				}
				log.Printf("%08x closed", ctx.Cid)
			}(ctx, cli)
		}
	}()

	return nil
}

// NewLocale returns a Locale.
func NewLocale(listen string, dialer Dialer) *Locale {
	return &Locale{
		Listen: listen,
		Dialer: dialer,
	}
}

// ============================================================================
//               ___           ___           ___           ___
//              /\  \         /\  \         /\  \         /\  \
//             /::\  \       /::\  \       /::\  \       /::\  \
//            /:/\:\  \     /:/\:\  \     /:/\:\  \     /:/\:\  \
//           /::\~\:\  \   /:/  \:\  \   /::\~\:\  \   /:/  \:\__\
//          /:/\:\ \:\__\ /:/__/ \:\__\ /:/\:\ \:\__\ /:/__/ \:|__|
//          \/_|::\/:/  / \:\  \ /:/  / \/__\:\/:/  / \:\  \ /:/  /
//             |:|::/  /   \:\  /:/  /       \::/  /   \:\  /:/  /
//             |:|\/__/     \:\/:/  /        /:/  /     \:\/:/  /
//             |:|  |        \::/  /        /:/  /       \::/__/
//              \|__|         \/__/         \/__/         ~~
// ============================================================================

// A Road represents a host's road mode.
type Road uint32

const (
	// RoadLocale means it don't need a proxy
	RoadLocale Road = iota
	// RoadRemote means it should accessed through proxy
	RoadRemote
	// RoadFucked means it is pure rubbish
	RoadFucked
	// RoadPuzzle means ?
	RoadPuzzle
)

func (r Road) String() string {
	switch r {
	case RoadLocale:
		return "direct"
	case RoadRemote:
		return "remote"
	case RoadFucked:
		return "fucked"
	case RoadPuzzle:
		return "puzzle"
	}
	panic("unreachable")
}

// Router is a selector that will judge the host address.
type Router interface {
	// The host must be a literal IP address, or a host name that can be resolved to IP addresses.
	// Examples:
	//	 Road("golang.org")
	//	 Road("192.0.2.1")
	Road(ctx *Context, host string) Road
}

// RouterIPNet is a router by IPNets. It judges whether an IP or domain name is within its range, if so, it returns Y,
// otherwise it returns N.
type RouterIPNet struct {
	L []*net.IPNet
	R Road
}

// Road implements daze.Router.
func (r *RouterIPNet) road(ctx *Context, host string) Road {
	if len(r.L) == 0 {
		return RoadPuzzle
	}
	l, err := Conf.Dialer.Resolver.LookupIPAddr(context.Background(), host)
	if err != nil {
		log.Printf("%08x  error %s", ctx.Cid, err)
		return RoadPuzzle
	}
	if len(l) == 0 {
		return RoadPuzzle
	}
	a := l[0]
	for _, e := range r.L {
		if e.Contains(a.IP) {
			return r.R
		}
	}
	return RoadPuzzle
}

// Road implements daze.Router.
func (r *RouterIPNet) Road(ctx *Context, host string) Road {
	road := r.road(ctx, host)
	log.Printf("%08x  route router=ipnet road=%s", ctx.Cid, road)
	return road
}

// FromReader loads a CIDR file from reader.
func (r *RouterIPNet) FromReader(f io.Reader) error {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		_, cidr, err := net.ParseCIDR(line)
		doa.Nil(err)
		r.L = append(r.L, cidr)
	}
	return scanner.Err()
}

// NewRouterIPNet returns a new RouterIPNet object.
func NewRouterIPNet(ipnets []*net.IPNet, road Road) *RouterIPNet {
	return &RouterIPNet{
		L: ipnets,
		R: road,
	}
}

// RouterRight always returns the same road.
type RouterRight struct {
	R Road
}

// Road implements daze.Router.
func (r *RouterRight) Road(ctx *Context, host string) Road {
	log.Printf("%08x  route router=right road=%s", ctx.Cid, r.R)
	return r.R
}

// NewRouterRight returns a new RouterRight.
func NewRouterRight(road Road) *RouterRight {
	return &RouterRight{R: road}
}

// NewRouterLocal returns reserved ip addresses.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/Reserved_IP_addresses
func NewRouterLocal() *RouterIPNet {
	r := []*net.IPNet{}
	for _, e := range [][2]string{
		// IPv4
		{"00000000", "FF000000"},
		{"0A000000", "FF000000"},
		{"7F000000", "FF000000"},
		{"A9FE0000", "FFFF0000"},
		{"AC100000", "FFF00000"},
		{"C0000000", "FFFFFFF8"},
		{"C00000AA", "FFFFFFFE"},
		{"C0000200", "FFFFFF00"},
		{"C0A80000", "FFFF0000"},
		{"C6120000", "FFFE0000"},
		{"C6336400", "FFFFFF00"},
		{"CB007100", "FFFFFF00"},
		{"F0000000", "F0000000"},
		{"FFFFFFFF", "FFFFFFFF"},
		// IPv6
		{"00000000000000000000000000000000", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"},
		{"00000000000000000000000000000001", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"},
		{"01000000000000000000000000000000", "FFFFFFFFFFFFFFFF0000000000000000"},
		{"0064FF9B000000000000000000000000", "FFFFFFFFFFFFFFFFFFFFFFFF00000000"},
		{"20010000000000000000000000000000", "FFFFFFFF000000000000000000000000"},
		{"20010010000000000000000000000000", "FFFFFFF0000000000000000000000000"},
		{"20010020000000000000000000000000", "FFFFFFF0000000000000000000000000"},
		{"20010DB8000000000000000000000000", "FFFFFFFF000000000000000000000000"},
		{"20020000000000000000000000000000", "FFFF0000000000000000000000000000"},
		{"FC000000000000000000000000000000", "FE000000000000000000000000000000"},
		{"FE800000000000000000000000000000", "FFC00000000000000000000000000000"},
		{"FF000000000000000000000000000000", "FF000000000000000000000000000000"},
	} {
		i := doa.Try(hex.DecodeString(e[0]))
		m := doa.Try(hex.DecodeString(e[1]))
		r = append(r, &net.IPNet{IP: i, Mask: m})
	}
	return NewRouterIPNet(r, RoadLocale)
}

// RouterCache cache routing results for next use.
type RouterCache struct {
	Pit Router
	Box *lru.Lru[string, Road]
	m   sync.Mutex
}

// Road implements daze.Router.
func (r *RouterCache) road(ctx *Context, host string) Road {
	if a, b := r.Box.GetExists(host); b {
		return a
	}
	a := r.Pit.Road(ctx, host)
	r.Box.Set(host, a)
	return a
}

// Road implements daze.Router.
func (r *RouterCache) Road(ctx *Context, host string) Road {
	r.m.Lock()
	defer r.m.Unlock()
	road := r.road(ctx, host)
	log.Printf("%08x  route router=cache road=%s", ctx.Cid, road)
	return road
}

// NewRouterCache returns a new Cache object.
func NewRouterCache(r Router) *RouterCache {
	return &RouterCache{
		Pit: r,
		Box: lru.New[string, Road](Conf.RouterCache),
		m:   sync.Mutex{},
	}
}

// RouterClump concat multiple routers in series.
type RouterClump struct {
	L []Router
}

// Road implements daze.Router.
func (r *RouterClump) road(ctx *Context, host string) Road {
	var a Road
	for _, e := range r.L {
		a = e.Road(ctx, host)
		if a != RoadPuzzle {
			return a
		}
	}
	return RoadPuzzle
}

// Road implements daze.Router.
func (r *RouterClump) Road(ctx *Context, host string) Road {
	road := r.road(ctx, host)
	log.Printf("%08x  route router=clump road=%s", ctx.Cid, road)
	return road
}

// NewRouterClump returns a new RouterClump.
func NewRouterClump(router ...Router) *RouterClump {
	return &RouterClump{
		L: router,
	}
}

// RouterRules aims to be a minimal configuration file format that's easy to read due to obvious semantics.
// There are two parts per line on RULE file: mode and glob. mode are on the left of the space sign and glob are on the
// right. mode is an char and describes whether the host should go proxy, glob supported glob-style patterns:
//
//   h?llo matches hello, hallo and hxllo
//   h*llo matches hllo and heeeello
//   h[ae]llo matches hello and hallo, but not hillo
//   h[^e]llo matches hallo, hbllo, ... but not hello
//   h[a-b]llo matches hallo and hbllo
//
// This is a RULE document:
//   L a.com a.a.com
//   R b.com *.b.com
//   B c.com
//
// L(ocale)  means using locale network
// R(emote)  means using remote network
// B(anned)  means block it
type RouterRules struct {
	L []string
	R []string
	B []string
}

// Road implements daze.Router.
func (r *RouterRules) road(ctx *Context, host string) Road {
	for _, e := range r.L {
		if doa.Try(filepath.Match(e, host)) {
			return RoadLocale
		}
	}
	for _, e := range r.R {
		if doa.Try(filepath.Match(e, host)) {
			return RoadRemote
		}
	}
	for _, e := range r.B {
		if doa.Try(filepath.Match(e, host)) {
			return RoadFucked
		}
	}
	return RoadPuzzle
}

// Road implements daze.Router.
func (r *RouterRules) Road(ctx *Context, host string) Road {
	road := r.road(ctx, host)
	log.Printf("%08x  route router=rules road=%s", ctx.Cid, road)
	return road
}

// FromReader loads a RULE file from reader.
func (r *RouterRules) FromReader(f io.Reader) error {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		seps := strings.Fields(line)
		if len(seps) < 2 {
			continue
		}
		switch seps[0] {
		case "#":
		case "L":
			r.L = append(r.L, seps[1:]...)
		case "R":
			r.R = append(r.R, seps[1:]...)
		case "B":
			r.B = append(r.B, seps[1:]...)
		}
	}
	return scanner.Err()
}

// NewRouterRules returns a new RoaderRules.
func NewRouterRules() *RouterRules {
	return &RouterRules{
		L: []string{},
		R: []string{},
		B: []string{},
	}
}

// Aimbot automatically distinguish whether to use a proxy or a local network.
type Aimbot struct {
	Remote Dialer
	Locale Dialer
	Router Router
}

// Dial connects to the address on the named network.
func (s *Aimbot) Dial(ctx *Context, network string, address string) (io.ReadWriteCloser, error) {
	var (
		dst string
		err error
		rwc io.ReadWriteCloser
	)
	log.Printf("%08x   dial network=%s address=%s", ctx.Cid, network, address)
	dst, _, err = net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	switch s.Router.Road(ctx, dst) {
	case RoadLocale:
		rwc, err = s.Locale.Dial(ctx, network, address)
	case RoadRemote:
		rwc, err = s.Remote.Dial(ctx, network, address)
	case RoadFucked:
		err = fmt.Errorf("daze: %s has been blocked", dst)
	case RoadPuzzle:
		rwc, err = s.Remote.Dial(ctx, network, address)
	}
	if err == nil {
		log.Printf("%08x  estab", ctx.Cid)
	}
	return rwc, err
}

// ============================================================================
//               ___           ___           ___           ___
//              /\  \         /\  \         /\  \         /\__\
//              \:\  \       /::\  \       /::\  \       /:/  /
//               \:\  \     /:/\:\  \     /:/\:\  \     /:/  /
//               /::\  \   /:/  \:\  \   /:/  \:\  \   /:/  /
//              /:/\:\__\ /:/__/ \:\__\ /:/__/ \:\__\ /:/__/
//             /:/  \/__/ \:\  \ /:/  / \:\  \ /:/  / \:\  \
//            /:/  /       \:\  /:/  /   \:\  /:/  /   \:\  \
//           /:/  /         \:\/:/  /     \:\/:/  /     \:\  \
//          /:/  /           \::/  /       \::/  /       \:\__\
//          \/__/             \/__/         \/__/         \/__/
// ============================================================================

// Check interface implementation.
var (
	_ Dialer = (*Aimbot)(nil)
	_ Dialer = (*Direct)(nil)
	_ Router = (*RouterCache)(nil)
	_ Router = (*RouterClump)(nil)
	_ Router = (*RouterIPNet)(nil)
	_ Router = (*RouterRight)(nil)
	_ Router = (*RouterRules)(nil)
)

// GravityReader wraps an io.Reader with RC4 crypto.
func GravityReader(r io.Reader, k []byte) io.Reader {
	cr := doa.Try(rc4.NewCipher(k))
	return cipher.StreamReader{S: cr, R: r}
}

// GravityWriter wraps an io.Writer with RC4 crypto.
func GravityWriter(w io.Writer, k []byte) io.Writer {
	cw := doa.Try(rc4.NewCipher(k))
	return cipher.StreamWriter{S: cw, W: w}
}

// Gravity double, happiness double.
func Gravity(conn io.ReadWriteCloser, k []byte) io.ReadWriteCloser {
	cr := doa.Try(rc4.NewCipher(k))
	cw := doa.Try(rc4.NewCipher(k))
	return &ReadWriteCloser{
		Reader: cipher.StreamReader{S: cr, R: conn},
		Writer: cipher.StreamWriter{S: cw, W: conn},
		Closer: conn,
	}
}

// Hang prevent program from exiting.
func Hang() {
	select {}
}

// OpenFile select the appropriate method to open the file based on the incoming args automatically.
//
// Examples:
//   OpenFile("/etc/hosts")
//   OpenFile("https://raw.githubusercontent.com/mohanson/daze/master/README.md")
func OpenFile(name string) (io.ReadCloser, error) {
	if strings.HasPrefix(name, "http://") || strings.HasPrefix(name, "https://") {
		resp, err := http.Get(name)
		if err != nil {
			return nil, err
		}
		return resp.Body, nil
	}
	return os.Open(name)
}

// A Priority is a mutual exclusion lock with priority.
type Priority struct {
	mu []*sync.Mutex
}

// Priority locks m with priority n, execute f at pass.
func (p *Priority) Priority(n int, f func() error) error {
	m := len(p.mu) - 1
	for i := n; i <= m; i++ {
		p.mu[i].Lock()
	}
	r := f()
	for i := m; i >= n; i-- {
		p.mu[i].Unlock()
	}
	return r
}

// NewPriority returns a new Priority.
func NewPriority(n int) *Priority {
	mu := make([]*sync.Mutex, n)
	for i := 0; i < n; i++ {
		mu[i] = &sync.Mutex{}
	}
	return &Priority{
		mu: mu,
	}
}

// Salt converts the stupid password passed in by the user to 128-sized byte array.
func Salt(s string) []byte {
	r := make([]byte, 128)
	h := md5.Sum([]byte(s))
	for i := 0; i < 8; i++ {
		copy(r[0x10*i:0x10*i+0x10], h[:])
	}
	return r
}

// ============================================================================
//                 ___           ___           ___           ___
//                /\  \         /\  \         /\  \         /\  \
//               /::\  \       /::\  \        \:\  \       /::\  \
//              /:/\:\  \     /:/\:\  \        \:\  \     /:/\:\  \
//             /:/  \:\__\   /::\~\:\  \       /::\  \   /::\~\:\  \
//            /:/__/ \:|__| /:/\:\ \:\__\     /:/\:\__\ /:/\:\ \:\__\
//            \:\  \ /:/  / \/__\:\/:/  /    /:/  \/__/ \/__\:\/:/  /
//             \:\  /:/  /       \::/  /    /:/  /           \::/  /
//              \:\/:/  /        /:/  /     \/__/            /:/  /
//               \::/__/        /:/  /                      /:/  /
//                ~~            \/__/                       \/__/
// ============================================================================

// LoadApnic loads remote resource. APNIC is the Regional Internet Registry administering IP addresses for the Asia
// Pacific.
func LoadApnic() map[string][]*net.IPNet {
	f := doa.Try(OpenFile("http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest"))
	defer f.Close()
	r := map[string][]*net.IPNet{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		seps := strings.Split(line, "|")
		if seps[1] == "*" {
			continue
		}
		switch seps[2] {
		case "ipv4":
			sep4 := doa.Try(strconv.ParseUint(seps[4], 0, 32))
			// Determine whether it is a power of 2
			doa.Doa(sep4&(sep4-1) == 0)
			mask := bits.LeadingZeros64(sep4) - 31
			_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%d", seps[3], mask))
			doa.Nil(err)
			r[seps[1]] = append(r[seps[1]], cidr)
		case "ipv6":
			seps := strings.Split(line, "|")
			sep4 := seps[4]
			_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%s", seps[3], sep4))
			doa.Nil(err)
			r[seps[1]] = append(r[seps[1]], cidr)
		}
	}
	return r
}
