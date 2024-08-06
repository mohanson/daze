package etch

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"math"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
	"github.com/mohanson/daze/protocol/ashe"
	"golang.org/x/net/quic"
)

// Stream is an ordered byte stream. Reads are buffered, writes are not buffered.
type Stream struct {
	*quic.Stream
}

// Write implements io.Writer.
func (c *Stream) Write(p []byte) (int, error) {
	n, err := c.Stream.Write(p)
	c.Stream.Flush()
	return n, err
}

// NewStream returns a new stream.
func NewStream(s *quic.Stream) *Stream {
	return &Stream{s}
}

// Server implemented the etch protocol.
type Server struct {
	Cipher []byte
	Closer *quic.Endpoint
	Listen string
}

// Close listener. Calling this function will disconnect all connections.
func (s *Server) Close() error {
	if s.Closer != nil {
		return s.Closer.Close(context.Background())
	}
	return nil
}

// Serve incoming connections. Parameter cli will be closed automatically when the function exits.
func (s *Server) Serve(ctx *daze.Context, cli io.ReadWriteCloser) error {
	spy := &ashe.Server{Cipher: s.Cipher}
	return spy.Serve(ctx, cli)
}

// Run it.
func (s *Server) Run() error {
	l, err := quic.Listen("udp", s.Listen, &quic.Config{
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{NewCert()},
			MinVersion:   tls.VersionTLS13,
		},
		MaxIdleTimeout: -1,
	})
	if err != nil {
		return err
	}
	s.Closer = l
	log.Println("main: listen and serve on", s.Listen)

	go func() {
		idx := uint32(math.MaxUint32)
		for {
			mux, err := l.Accept(context.Background())
			if err != nil {
				log.Println("main:", err)
				break
			}
			go func() {
				defer mux.Close()
				for {
					cli, err := mux.AcceptStream(context.Background())
					if err != nil {
						log.Println("main:", err)
						break
					}
					idx++
					ctx := &daze.Context{Cid: idx}
					log.Printf("conn: %08x accept", ctx.Cid)
					go func() {
						defer cli.Close()
						if err := s.Serve(ctx, NewStream(cli)); err != nil {
							log.Printf("conn: %08x  error %s", ctx.Cid, err)
						}
						log.Printf("conn: %08x closed", ctx.Cid)
					}()
				}
			}()
		}
	}()
	return nil
}

// NewServer returns a new Server.
func NewServer(listen string, cipher string) *Server {
	return &Server{
		Cipher: daze.Salt(cipher),
		Listen: listen,
	}
}

// Client implemented the etch protocol.
type Client struct {
	Cipher []byte
	Quicon *quic.Conn
	Server string
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	srv, err := c.Quicon.NewStream(context.Background())
	if err != nil {
		return nil, err
	}
	spy := &ashe.Client{Cipher: c.Cipher}
	con, err := spy.Estab(ctx, NewStream(srv), network, address)
	return con, err
}

// NewClient returns a new Client. A secret data needs to be passed in Cipher, as a sign to interface with the Server.
func NewClient(server, cipher string) *Client {
	quiced := doa.Try(quic.Listen("udp", ":0", nil))
	quicon := doa.Try(quiced.Dial(context.Background(), "udp", server, &quic.Config{
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
		},
		MaxIdleTimeout: -1,
	}))
	return &Client{
		Cipher: daze.Salt(cipher),
		Quicon: quicon,
		Server: server,
	}
}
