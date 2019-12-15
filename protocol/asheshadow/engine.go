package asheshadow

import (
	"crypto/md5"
	"encoding/base64"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
)

// This protocol is an upgraded version of ashe, which uses the HTTP
// obfuscation mechanism.

var (
	prefix = func() string {
		ls := []string{
			"HTTP/1.1 400 Bad Request",
			"Content-Length: 0",
			"Content-Type: text/plain; charset=utf-8",
			"Date: Tue, 06 Feb 2018 15:46:24 GMT",
			"X-Content-Type-Options: nosniff",
		}
		return strings.Join(ls, "\r\n") + "\r\n"
	}()
)

type Server struct {
	Listen string
	Masker string
	Origin *ashe.Server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	date := r.Header.Get("Date")
	hash := md5.Sum(append([]byte(date), s.Origin.Cipher[:]...))
	if r.Header.Get("Content-MD5") == base64.StdEncoding.EncodeToString(hash[:]) {
		hj, _ := w.(http.Hijacker)
		cc, rw, _ := hj.Hijack()
		defer cc.Close()
		defer rw.Flush()

		sr := strings.Replace(prefix, "Tue, 06 Feb 2018 15:46:24 GMT", time.Now().Format(time.RFC1123), 1)
		cc.Write([]byte(sr))

		connl := &daze.ReadWriteCloser{
			Reader: rw,
			Writer: cc,
			Closer: cc,
		}
		if err := s.Origin.Serve(connl); err != nil {
			log.Println(err)
		}
		return
	}
	q, err := http.NewRequest(r.Method, s.Masker+r.RequestURI, r.Body)
	if err != nil {
		return
	}
	q.Header = r.Header
	p, err := http.DefaultClient.Do(q)
	if err != nil {
		return
	}
	defer p.Body.Close()

	for k, v := range p.Header {
		for _, e := range v {
			w.Header().Add(k, e)
		}
	}
	w.WriteHeader(p.StatusCode)
	io.Copy(w, p.Body)
}

func (s *Server) Run() error {
	log.Println("Listen and serve on", s.Listen)
	return http.ListenAndServe(s.Listen, s)
}

func NewServer(listen, cipher string) *Server {
	return &Server{
		Listen: listen,
		Masker: "http://httpbin.org",
		Origin: &ashe.Server{
			Cipher: md5.Sum([]byte(cipher)),
		},
	}
}

type Client struct {
	Server string
	Origin *ashe.Client
}

func (c *Client) Dial(network string, address string) (io.ReadWriteCloser, error) {
	var (
		conn io.ReadWriteCloser
		buf  = make([]byte, 1024)
		date string
		hash [16]byte
		req  *http.Request
		err  error
	)
	conn, err = net.DialTimeout("tcp", c.Server, time.Second*4)
	if err != nil {
		return nil, err
	}
	req, err = http.NewRequest("POST", "http://"+c.Server, http.NoBody)
	if err != nil {
		return nil, err
	}
	date = time.Now().Format(time.RFC1123)
	req.Header.Set("Date", date)
	hash = md5.Sum(append([]byte(date), c.Origin.Cipher[:]...))
	req.Header.Set("Content-MD5", base64.StdEncoding.EncodeToString(hash[:]))
	req.Write(conn)
	io.ReadFull(conn, buf[:len(prefix)])
	return c.Origin.DialConn(conn, network, address)
}

func NewClient(server, cipher string) *Client {
	return &Client{
		Server: server,
		Origin: &ashe.Client{
			Cipher: md5.Sum([]byte(cipher)),
		},
	}
}
