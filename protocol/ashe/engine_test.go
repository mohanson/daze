package ashe

import (
	"io"
	"testing"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
)

const (
	EchoServerListenOn = "127.0.0.1:28080"
	DazeServerListenOn = "127.0.0.1:28081"
	Password           = "password"
)

func TestProtocolAsheTCP(t *testing.T) {
	remote := daze.NewTester(EchoServerListenOn)
	defer remote.Close()
	remote.TCP()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	buf := make([]byte, 2048)
	doa.Try(cli.Write([]byte{0x00, 0x00, 0x00, 0x80}))
	doa.Try(io.ReadFull(cli, buf[:132]))
}

func TestProtocolAsheTCPClientClose(t *testing.T) {
	remote := daze.NewTester(EchoServerListenOn)
	defer remote.Close()
	remote.TCP()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	cli.Close()
	_, er1 := cli.Write([]byte{0x01, 0x00, 0x00, 0x00})
	doa.Doa(er1 != nil)
	buf := make([]byte, 2048)
	_, er2 := io.ReadFull(cli, buf[:1])
	doa.Doa(er2 != nil)
}

func TestProtocolAsheTCPServerClose(t *testing.T) {
	remote := daze.NewTester(EchoServerListenOn)
	defer remote.Close()
	remote.TCP()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	buf := make([]byte, 2048)
	doa.Try(cli.Write([]byte{0x01, 0x00, 0x00, 0x00}))
	_, err := io.ReadFull(cli, buf[:1])
	doa.Doa(err != nil)
}

func TestProtocolAsheUDP(t *testing.T) {
	remote := daze.NewTester(EchoServerListenOn)
	defer remote.Close()
	remote.UDP()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "udp", EchoServerListenOn))
	defer cli.Close()

	buf := make([]byte, 2048)
	doa.Try(cli.Write([]byte{0x00, 0x00, 0x00, 0x80}))
	doa.Try(io.ReadFull(cli, buf[:132]))
}
