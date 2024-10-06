package ashe

import (
	"encoding/binary"
	"io"
	"math/rand/v2"
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
	dazeRemote := daze.NewTester(EchoServerListenOn)
	defer dazeRemote.Close()
	dazeRemote.TCP()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	var (
		bsz = max(4, int(rand.Uint32N(256)))
		buf = make([]byte, bsz)
		cnt int
		rsz = int(rand.Uint32N(65536))
	)
	copy(buf[0:2], []byte{0x00, 0x00})
	binary.BigEndian.PutUint16(buf[2:], uint16(rsz))
	doa.Try(cli.Write(buf[:4]))
	cnt = 0
	for {
		e := min(rand.IntN(bsz+1), rsz-cnt)
		n := doa.Try(io.ReadFull(cli, buf[:e]))
		for i := range n {
			doa.Doa(buf[i] == 0x00)
		}
		cnt += n
		if cnt == rsz {
			break
		}
	}
	copy(buf[0:2], []byte{0x01, 0x00})
	binary.BigEndian.PutUint16(buf[2:], uint16(rsz))
	doa.Try(cli.Write(buf[:4]))
	for i := range bsz {
		buf[i] = 0x00
	}
	cnt = 0
	for {
		e := min(rand.IntN(bsz+1), rsz-cnt)
		n := doa.Try(cli.Write(buf[:e]))
		cnt += n
		if cnt == rsz {
			break
		}
	}
}

func TestProtocolAsheTCPClientClose(t *testing.T) {
	dazeRemote := daze.NewTester(EchoServerListenOn)
	defer dazeRemote.Close()
	dazeRemote.TCP()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	cli.Close()
	_, er1 := cli.Write([]byte{0x02, 0x00, 0x00, 0x00})
	doa.Doa(er1 != nil)
	buf := make([]byte, 2048)
	_, er2 := io.ReadFull(cli, buf[:1])
	doa.Doa(er2 != nil)
}

func TestProtocolAsheTCPServerClose(t *testing.T) {
	dazeRemote := daze.NewTester(EchoServerListenOn)
	defer dazeRemote.Close()
	dazeRemote.TCP()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	buf := make([]byte, 2048)
	doa.Try(cli.Write([]byte{0x02, 0x00, 0x00, 0x00}))
	_, err := io.ReadFull(cli, buf[:1])
	doa.Doa(err != nil)
}

func TestProtocolAsheUDP(t *testing.T) {
	dazeRemote := daze.NewTester(EchoServerListenOn)
	defer dazeRemote.Close()
	dazeRemote.UDP()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "udp", EchoServerListenOn))
	defer cli.Close()

	buf := make([]byte, 2048)
	doa.Try(cli.Write([]byte{0x00, 0x00, 0x00, 0x80}))
	doa.Try(io.ReadFull(cli, buf[:128]))
}
