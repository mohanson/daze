package main

import (
	"bytes"
	"context"
	"flag"
	"log"
	"net"
	"net/url"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
	"github.com/mohanson/daze/protocol/etch"
)

func ResolverDoe(addr string) *net.Resolver {
	urls := doa.Try(url.Parse(addr))
	host := doa.Try(net.LookupHost(urls.Hostname()))[0]
	port := urls.Port()
	urls.Host = host
	if port != "" {
		urls.Host = host + ":" + port
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			conn := &daze.WireConn{
				Call: func(b []byte) ([]byte, error) {
					return etch.NewClient(addr).Call("Dns.Wire", b)
				},
				Data: bytes.NewBuffer([]byte{}),
			}
			return conn, nil
		},
	}
}

func main() {
	flag.Parse()
	switch flag.Arg(0) {
	case "server":
		server := etch.NewServer("127.0.0.1:8080")
		server.Run()
		select {}
	case "client":
		dns := ResolverDoe("http://127.0.0.1:8080")
		log.Println(dns.LookupHost(context.Background(), "google.com"))
	}
}
