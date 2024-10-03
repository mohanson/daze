package etch

import (
	"io"
	"log"
	"net"

	"github.com/mohanson/daze/lib/doa"
)

// Conf is acting as package level configuration.
var Conf = struct {
	PayloadSize int
}{
	// The maximum safe udp payload is 508 bytes. This is a packet size of 576 (the minimum maximum reassembly buffer
	// size), minus the maximum 60-byte ip header and the 8-byte udp header.
	PayloadSize: 508,
}

// Server implemented the etch protocol.
type Server struct {
	Closer io.Closer
	Listen string
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
	addr, err := net.ResolveUDPAddr("udp", s.Listen)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return err
	}
	s.Closer = conn
	log.Println("main: listen and serve on", s.Listen)

	go func() {
		buf := make([]byte, Conf.PayloadSize)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				log.Println("main:", err)
				break
			}
			log.Println(addr, string(buf[:n]))
		}
	}()

	return nil
}

func NewServer(listen string) *Server {
	return &Server{
		Closer: nil,
		Listen: listen,
	}
}

type Client struct {
	Server string
}

func (s *Client) Run() {
	addr := doa.Try(net.ResolveUDPAddr("udp", s.Server))
	conn := doa.Try(net.DialUDP("udp", nil, addr))
	doa.Try(conn.Write([]byte("Hello World!")))
}

func NewClient(server string) *Client {
	return &Client{
		Server: server,
	}
}
