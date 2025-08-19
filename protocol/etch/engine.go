package etch

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"net/rpc/jsonrpc"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
)

// Dns represents a DNS resolver that communicates with a DNS server.
type Dns struct {
	Server string // URL of the DNS server (e.g., "https://1.1.1.1/dns-query")
}

// Wire processes a DNS query by sending it to the configured DNS server and returning the response.
func (d *Dns) Wire(args string, reply *string) error {
	params, err := base64.StdEncoding.DecodeString(args)
	if err != nil {
		return err
	}
	r, err := http.Post(d.Server, "application/dns-message", bytes.NewReader(params))
	if err != nil {
		return err
	}
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	*reply = base64.StdEncoding.EncodeToString(b)
	return nil
}

// Server implemented the etch protocol.
type Server struct {
	Closer io.Closer
	Listen string
}

// Close listener. Established connections will not be closed.
func (s *Server) Close() error {
	if s.Closer != nil {
		return s.Closer.Close()
	}
	return nil
}

// Run it.
func (s *Server) Run() error {
	rpc.Register(&Dns{
		Server: "https://1.1.1.1/dns-query",
	})
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

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rwc := daze.ReadWriteCloser{
		Reader: r.Body,
		Writer: w,
		Closer: r.Body,
	}
	codec := jsonrpc.NewServerCodec(rwc)
	rpc.ServeRequest(codec)
}

// NewServer returns a new Server.
func NewServer(listen string) *Server {
	return &Server{
		Closer: nil,
		Listen: listen,
	}
}

// Client implemented the etch protocol.
type Client struct {
	Server string
}

func (c *Client) Call(method string, args []byte) ([]byte, error) {
	sendJson := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  method,
		"params":  []string{base64.StdEncoding.EncodeToString(args)},
	}
	sendData := doa.Try(json.Marshal(sendJson))
	r, err := http.Post(c.Server, "application/json", bytes.NewBuffer(sendData))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	recvJson := map[string]any{}
	if err := json.NewDecoder(r.Body).Decode(&recvJson); err != nil {
		return nil, err
	}
	if recvJson["error"] != nil {
		return nil, fmt.Errorf("%s", recvJson["error"])
	}
	recvData, err := base64.StdEncoding.DecodeString(recvJson["result"].(string))
	if err != nil {
		return nil, err
	}
	return recvData, nil
}

// NewClient returns a new Client.
func NewClient(server string) *Client {
	return &Client{
		Server: server,
	}
}
