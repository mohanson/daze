package ashe

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"github.com/godump/doa"
	"github.com/mohanson/daze"
)

const (
	EchoServerListenOn = "127.0.0.1:28080"
	DazeServerListenOn = "127.0.0.1:21081"
	Password           = "password"
)

func TestProtocolAsheTCP(t *testing.T) {
	echoListener := doa.Try(net.Listen("tcp", EchoServerListenOn))
	defer echoListener.Close()
	go func() {
		for {
			c, err := echoListener.Accept()
			if err != nil {
				break
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(c)
		}
	}()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	go dazeServer.Run()

	time.Sleep(time.Second)

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{Cid: "00000000"}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	buf0 := []byte("Hello World!")
	cli.Write(buf0)
	buf1 := make([]byte, 12)
	io.ReadFull(cli, buf1)
	if !bytes.Equal(buf0, buf1) {
		t.FailNow()
	}
}

func TestProtocolAsheUDP(t *testing.T) {
	echoAddr := doa.Try(net.ResolveUDPAddr("udp", EchoServerListenOn))
	echoServer := doa.Try(net.ListenUDP("udp", echoAddr))
	defer echoServer.Close()
	go func() {
		b := make([]byte, 1024)
		for {
			n, addr, err := echoServer.ReadFromUDP(b)
			if err != nil {
				break
			}
			echoServer.WriteToUDP(b[:n], addr)
		}
	}()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	go dazeServer.Run()

	time.Sleep(time.Second)

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{Cid: "00000000"}
	cli := doa.Try(dazeClient.Dial(ctx, "udp", EchoServerListenOn))
	defer cli.Close()

	buf0 := []byte("Hello World!")
	cli.Write(buf0)
	buf1 := make([]byte, 12)
	io.ReadFull(cli, buf1)
	if !bytes.Equal(buf0, buf1) {
		t.FailNow()
	}
}
