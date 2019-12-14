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

// This protocol has obvious fingerprints, please modify the code here
// randomly after you read this advice. God bless you.
type Server struct {
	Listen string
	Masker string
	Origin *ashe.Server
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
	if r.Header.Get("Accept") == "application/daze" {
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
		if err := s.Origin.Serve(connl); err != nil {
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
		req  *http.Request
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

	rand.Read(buf[:8])
	name := hex.EncodeToString(buf[:8])
	req, err = http.NewRequest("POST", "http://"+c.Server+"/"+name, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/daze")
	req.Write(conn)
	io.ReadFull(conn, buf[:len(responsePrefix)])

	return c.Origin.Make(conn, network, address)
}

func NewClient(server, cipher string) *Client {
	return &Client{
		Server: server,
		Origin: &ashe.Client{
			Cipher: md5.Sum([]byte(cipher)),
		},
	}
}
