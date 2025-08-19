package etch

import (
	"io"
	"log"
	"net"
	"net/http"
)

// Server implemented the etch protocol.
type Server struct {
	Closer io.Closer
	Listen string
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println(w)
	log.Println(r)
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
func NewServer(listen string) *Server {
	return &Server{
		Listen: listen,
	}
}
