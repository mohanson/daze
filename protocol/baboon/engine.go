package baboon

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/godump/doa"
	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
)

// Protocol baboon disguise the ashe protocol through the HTTP protocol.

var Conf = struct {
	Masker string
}{
	Masker: "https://www.baidu.com",
}

// Server implemented the baboon protocol.
type Server struct {
	Listen string
	Cipher [16]byte
	Closer io.Closer
	Masker string
	NextID uint32
}

// ServeMask forward the request to a fake website. From the outside, the daze server looks like a normal website.
func (s *Server) ServeMask(w http.ResponseWriter, r *http.Request) {
	req, err := http.NewRequest(r.Method, s.Masker+r.RequestURI, r.Body)
	if err != nil {
		return
	}
	req.Header = r.Header
	ret, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	defer ret.Body.Close()
	for k, v := range ret.Header {
		for _, e := range v {
			w.Header().Add(k, e)
		}
	}
	w.WriteHeader(ret.StatusCode)
	io.Copy(w, ret.Body)
}

// ServeDaze. Degenerate HTTP protocol and run ashe protocol on it.
func (s *Server) ServeDaze(w http.ResponseWriter, r *http.Request) {
	hj, _ := w.(http.Hijacker)
	cc, rw, _ := hj.Hijack()
	defer cc.Close()
	defer rw.Flush()
	io.WriteString(cc, "HTTP/1.1 200 OK\r\n")                                        // 17
	io.WriteString(cc, "Content-Length: 0\r\n")                                      // 19
	io.WriteString(cc, "Content-Type: text/plain; charset=utf-8\r\n")                // 41
	io.WriteString(cc, fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123))) // 37
	io.WriteString(cc, "X-Content-Type-Options: nosniff\r\n")                        // 33
	app := &daze.ReadWriteCloser{
		Reader: rw,
		Writer: cc,
		Closer: cc,
	}
	srv := ashe.Server{
		Listen: s.Listen,
		Cipher: s.Cipher,
	}
	ctx := &daze.Context{Cid: daze.Hu32(atomic.AddUint32(&s.NextID, 1))}
	log.Printf("%s accept remote=%s", ctx.Cid, cc.RemoteAddr())
	if err := srv.Serve(ctx, app); err != nil {
		log.Println(ctx.Cid, " error", err)
	}
	log.Println(ctx.Cid, "closed")
}

// Route check the type of a HTTP request.
func (s *Server) Route(r *http.Request) int {
	authText := r.Header.Get("Authorization")
	if authText == "" {
		return 0
	}
	authData, err := hex.DecodeString(authText)
	if err != nil {
		return 0
	}
	hash := md5.New()
	hash.Write(authData[:16])
	hash.Write(s.Cipher[:])
	sign := hash.Sum(nil)
	for i := 0; i < 16; i++ {
		if authData[16+i] != sign[i] {
			return 0
		}
	}
	return 1
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch s.Route(r) {
	case 0:
		s.ServeMask(w, r)
	case 1:
		s.ServeDaze(w, r)
	}
}

// Close listener.
func (s *Server) Close() error {
	if s.Closer != nil {
		return s.Closer.Close()
	}
	return nil
}

// Run.
func (s *Server) Run() error {
	log.Println("listen and serve on", s.Listen)
	srv := &http.Server{Addr: s.Listen, Handler: s}
	s.Closer = srv
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// NewServer returns a new Server. A secret data needs to be passed in Cipher, as a sign to interface with the Client.
func NewServer(listen string, cipher string) *Server {
	return &Server{
		Listen: listen,
		Cipher: md5.Sum([]byte(cipher)),
		Masker: Conf.Masker,
		NextID: uint32(math.MaxUint32),
	}
}

// Client implemented the ashe protocol.
type Client struct {
	Server string
	Cipher [16]byte
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	var (
		srv io.ReadWriteCloser
		buf = make([]byte, 256)
		req *http.Request
		err error
	)
	srv, err = daze.Conf.Dialer.Dial("tcp", c.Server)
	if err != nil {
		return nil, err
	}
	daze.Conf.Random.Read(buf[:16])
	copy(buf[16:32], c.Cipher[:])
	sign := md5.Sum(buf[:32])
	copy(buf[16:32], sign[:])
	req = doa.Try(http.NewRequest("POST", "http://"+c.Server+"/sync", http.NoBody))
	req.Header.Set("Authorization", hex.EncodeToString(buf[:32]))
	req.Write(srv)
	// Discard responded header
	io.ReadFull(srv, buf[:147])
	cli := &ashe.Client{
		Server: c.Server,
		Cipher: c.Cipher,
	}
	ret, err := cli.DialDaze(ctx, srv, network, address)
	if err != nil {
		srv.Close()
	}
	return ret, err
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher, as a sign to interface with the Server.
func NewClient(server string, cipher string) *Client {
	return &Client{
		Server: server,
		Cipher: md5.Sum([]byte(cipher)),
	}
}
