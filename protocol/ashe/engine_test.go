package ashe

import (
	"bytes"
	"errors"
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
				if errors.Is(err, net.ErrClosed) {
					break
				}
				continue
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
	c, _ := dazeClient.Dial(ctx, "tcp", EchoServerListenOn)
	defer c.Close()

	buf0 := []byte("Hello World!")
	c.Write(buf0)
	buf1 := make([]byte, 12)
	io.ReadFull(c, buf1)
	if !bytes.Equal(buf0, buf1) {
		t.FailNow()
	}
}

func TestProtocolAsheUDP(t *testing.T) {
	echoAddr, _ := net.ResolveUDPAddr("udp", EchoServerListenOn)
	echoServer, _ := net.ListenUDP("udp", echoAddr)
	defer echoServer.Close()

	go func() {
		for {
			b := make([]byte, 12)
			n, addr, _ := echoServer.ReadFromUDP(b)
			echoServer.WriteToUDP(b[:n], addr)
		}
	}()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	go dazeServer.Run()

	time.Sleep(time.Second)

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := &daze.Context{Cid: "00000000"}
	c, _ := dazeClient.Dial(ctx, "udp", EchoServerListenOn)
	defer c.Close()

	buf0 := []byte("Hello World!")
	c.Write(buf0)
	buf1 := make([]byte, 12)
	io.ReadFull(c, buf1)
	if !bytes.Equal(buf0, buf1) {
		t.FailNow()
	}
}
