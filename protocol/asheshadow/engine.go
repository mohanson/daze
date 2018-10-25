package asheshadow

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mohanson/daze"
)

type Server struct {
	Listen string
	Cipher [16]byte
	Masker string
}

func (s *Server) ServeDaze(connl io.ReadWriteCloser) error {
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
	log.Println("Connect[asheshadow]", dst)
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

var responsePrefix = func() string {
	ls := []string{
		"HTTP/1.1 400 Bad Request",
		"Content-Length: 0",
		"Content-Type: text/plain; charset=utf-8",
		"Date: Tue, 06 Feb 2018 15:46:24 GMT",
		"X-Content-Type-Options: nosniff",
	}
	return strings.Join(ls, "\r\n") + "\r\n"
}()

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("From") == "daze" {
		hj, _ := w.(http.Hijacker)
		cc, rw, _ := hj.Hijack()
		defer cc.Close()
		defer rw.Flush()

		sr := strings.Replace(responsePrefix, "Tue, 06 Feb 2018 15:46:24 GMT", time.Now().Format(time.RFC1123), 1)
		cc.Write([]byte(sr))

		connl := &daze.ReadWriteCloser{
			Reader: rw,
			Writer: cc,
			Closer: cc,
		}
		if err := s.ServeDaze(connl); err != nil {
			log.Println(err)
		}
		return
	}
	req2, err := http.NewRequest(r.Method, s.Masker+r.RequestURI, r.Body)
	if err != nil {
		return
	}
	req2.Header = r.Header
	resp, err := http.DefaultClient.Do(req2)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	for k, v := range resp.Header {
		for _, e := range v {
			w.Header().Add(k, e)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (s *Server) Run() error {
	log.Println("Listen and serve on", s.Listen)
	return http.ListenAndServe(s.Listen, s)
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
	if len(address) > 256 {
		return nil, fmt.Errorf("daze: destination address too long: %s", address)
	}
	var (
		conn io.ReadWriteCloser
		buf  = make([]byte, 1024)
		req  *http.Request
		err  error
		n    = len(address)
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

	rand.Read(buf[:4])
	name := hex.EncodeToString(buf[:4])
	req, err = http.NewRequest("POST", "http://"+c.Server+"/bin/exchange?name="+name, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("From", "daze")
	req.Write(conn)
	io.ReadFull(conn, buf[:len(responsePrefix)])

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
