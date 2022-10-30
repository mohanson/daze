package czar

import (
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"time"

	"github.com/godump/doa"
	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
)

// The czar protocol is a proxy protocol built on TCP multiplexing technology. By establishing multiple TCP connections
// in one TCP channel, czar protocol effectively reduces the consumption of establishing connections between the client
// and the server:
//
// Client port: a.com ------------┐                   ┌------------ Server port: a.com
// Client port: b.com ----------┐ |                   | ┌---------- Server port: b.com
// Client port: c.com ----------+-+-- czar protocol --+-+---------- Server port: c.com
// Client port: d.com ----------┘ |                   | └---------- Server port: d.com
// Client port: e.com ------------┘                   └------------ Server port: e.com
//
// To open a stream:
//
// +-----+-----+-----+-----+
// | Sid |  0  |    Rsv    |
// +-----+-----+-----+-----+
//
// Both server and client can push data to each other.
//
// +-----+-----+-----+-----+-----+-----+
// | Sid |  1  |    Len    |    Msg    |
// +-----+-----+-----+-----+-----+-----+
//
// Close the specified stream.
//
// +-----+-----+-----+-----+
// | Sid |  2  |    Rsv    |
// +-----+-----+-----+-----+

// Server implemented the czar protocol.
type Server struct {
	Listen string
	Cipher []byte
	Closer io.Closer
}

// Serve incoming connections. Parameter cli will be closed automatically when the function exits.
func (s *Server) Serve(ctx *daze.Context, cli io.ReadWriteCloser) error {
	asheServer := &ashe.Server{Cipher: s.Cipher}
	return asheServer.Serve(ctx, cli)
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
	s.Closer = l
	log.Println("main: listen and serve on", s.Listen)

	go func() {
		idx := uint32(math.MaxUint32)
		for {
			cli, err := l.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Println("main:", err)
				}
				break
			}
			mux := NewMuxServer(cli)
			go func(mux *Mux) {
				defer mux.Close()
				for cli := range mux.Accept() {
					idx++
					ctx := &daze.Context{Cid: idx}
					log.Printf("conn: %08x accept remote=%s", ctx.Cid, mux.conn.RemoteAddr())
					go func(cli io.ReadWriteCloser) {
						defer cli.Close()
						if err := s.Serve(ctx, cli); err != nil {
							log.Printf("conn: %08x  error %s", ctx.Cid, err)
						}
						log.Printf("conn: %08x closed", ctx.Cid)
					}(cli)
				}
			}(mux)
		}
	}()

	return nil
}

// NewServer returns a new Server.
func NewServer(listen string, cipher string) *Server {
	return &Server{
		Listen: listen,
		Cipher: daze.Salt(cipher),
	}
}

// Client implemented the czar protocol.
type Client struct {
	Cipher []byte
	Server string
	Mux    chan *Mux
}

func (c *Client) Close() error {
	return nil
}

// Dial connects to the address on the named network.
func (c *Client) Dial(ctx *daze.Context, network string, address string) (io.ReadWriteCloser, error) {
	select {
	case mux := <-c.Mux:
		srv, err := mux.Open()
		if err != nil {
			return nil, err
		}
		asheClient := &ashe.Client{Cipher: c.Cipher}
		ret, err := asheClient.With(ctx, srv, network, address)
		if err != nil {
			srv.Close()
		}
		return ret, err
	case <-time.After(daze.Conf.DialerTimeout):
		return nil, fmt.Errorf("dial tcp: %s: i/o timeout", address)
	}
}

// Run creates an establish connection to czar server.
func (c *Client) Run() {
	for {
		srv := doa.Try(daze.Reno("tcp", c.Server))
		log.Println("czar: mux init")
		mux := NewMuxClient(srv)
		for {
			select {
			case c.Mux <- mux:
				continue
			case <-mux.done:
			}
			log.Println("czar: mux done")
			break
		}
	}
}

// NewClient returns a new Client.
func NewClient(server, cipher string) *Client {
	client := &Client{
		Cipher: daze.Salt(cipher),
		Server: server,
		Mux:    make(chan *Mux),
	}
	go client.Run()
	return client
}
