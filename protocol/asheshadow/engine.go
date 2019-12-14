package asheshadow

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
)

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
	header = "Accept"
	symbol = "application/daze"
)

// This protocol has obvious fingerprints, please modify the code here
// randomly after you read this advice. God bless you.
type Server struct {
	Listen string
	Masker string
	Origin *ashe.Server
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get(header) == symbol {
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
		serv io.ReadWriteCloser
		kill *time.Timer
		buf  = make([]byte, 1024)
		req  *http.Request
		err  error
	)
	conn, err = net.DialTimeout("tcp", c.Server, time.Second*4)
	if err != nil {
		return nil, err
	}
	kill = time.AfterFunc(4*time.Second, func() {
		conn.Close()
	})
	_, err = rand.Read(buf[:8])
	if err != nil {
		return nil, err
	}
	req, err = http.NewRequest("POST", "http://"+c.Server+"/"+hex.EncodeToString(buf[:8]), http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set(header, symbol)
	req.Write(conn)
	io.ReadFull(conn, buf[:len(prefix)])
	serv, err = c.Origin.DialConn(conn, network, address)
	if err != nil {
		return nil, err
	}
	kill.Stop()
	return serv, nil
}

func NewClient(server, cipher string) *Client {
	return &Client{
		Server: server,
		Origin: &ashe.Client{
			Cipher: md5.Sum([]byte(cipher)),
		},
	}
}
