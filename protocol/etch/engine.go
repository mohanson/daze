package etch

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/mohanson/daze/lib/doa"
)

// Server implemented the etch protocol.
type Server struct {
	Closer io.Closer
	Listen string
	Server string
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cloud := doa.Try(url.Parse(s.Server))
	proxy := httputil.NewSingleHostReverseProxy(cloud)
	proxy.Director = nil
	proxy.Rewrite = func(r *httputil.ProxyRequest) {
		r.SetURL(cloud)
	}
	proxy.ServeHTTP(w, r)
}

// Close listener.
func (s *Server) Close() error {
	if s.Closer != nil {
		return s.Closer.Close()
	}
	return nil
}

// Run it.
func (s *Server) Run() error {
	l, err := net.Listen("tcp", s.Listen)
	if err != nil {
		return err
	}
	log.Println("main: listen and serve on", s.Listen)
	srv := &http.Server{Handler: s}
	s.Closer = srv
	go srv.Serve(l)
	return nil
}

// NewServer returns a new Server.
func NewServer(listen string, server string) *Server {
	return &Server{
		Closer: nil,
		Listen: listen,
		Server: server,
	}
}
