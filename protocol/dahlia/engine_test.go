package dahlia

import (
	"io"
	"testing"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
)

const (
	EchoServerListenOn = "127.0.0.1:28080"
	DazeServerListenOn = "127.0.0.1:28081"
	DazeClientListenOn = "127.0.0.1:21082"
	Password           = "password"
)

func TestProtocolDahliaTCP(t *testing.T) {
	remote := daze.NewTester(EchoServerListenOn)
	defer remote.Close()
	remote.TCP()

	dazeServer := NewServer(DazeServerListenOn, EchoServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeClientListenOn, DazeServerListenOn, Password)
	defer dazeClient.Close()
	dazeClient.Run()

	cli := doa.Try(daze.Dial("tcp", DazeClientListenOn))
	defer cli.Close()
	buf := make([]byte, 2048)
	doa.Try(cli.Write([]byte{0x00, 0x00, 0x00, 0x80}))
	doa.Try(io.ReadFull(cli, buf[:132]))
}
