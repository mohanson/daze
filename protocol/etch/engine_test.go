package etch

import (
	"context"
	"testing"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
)

const (
	DazeServerListenOn = "127.0.0.1:28080"
	Dest               = "https://1.1.1.1"
	HostLookup         = "google.com"
)

func TestProtocolEtch(t *testing.T) {
	server := NewServer(DazeServerListenOn, Dest)
	server.Run()
	defer server.Close()

	client := daze.ResolverDoh("http://127.0.0.1:28080/dns-query")
	doa.Try(client.LookupHost(context.Background(), HostLookup))
}
