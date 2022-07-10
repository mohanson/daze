package crow

import (
	"bytes"
	"errors"
	"io"
	"log"
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

func TestProtocalCrowTCP(t *testing.T) {
	defer time.Sleep(time.Second)
	echoListener := doa.Try(net.Listen("tcp", EchoServerListenOn))
	defer echoListener.Close()
	go func() {
		for {
			c, err := echoListener.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Println(err)
				}
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
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
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

func TestProtocalCrowTCPServerClose(t *testing.T) {
	defer time.Sleep(time.Second)
	echoListener := doa.Try(net.Listen("tcp", EchoServerListenOn))
	defer echoListener.Close()
	go func() {
		for {
			c, err := echoListener.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Println(err)
				}
				break
			}
			go func(c net.Conn) {
				defer c.Close()
			}(c)
		}
	}()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
	ctx := &daze.Context{Cid: "00000000"}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	buf0 := []byte("Hello World!")
	cli.Write(buf0)
	buf1 := make([]byte, 12)
	_, err := io.ReadFull(cli, buf1)
	if err == io.ErrUnexpectedEOF {
		t.FailNow()
	}
}

func TestProtocalCrowTCPClientDialFailed(t *testing.T) {
	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
	ctx := &daze.Context{Cid: "00000000"}
	_, err := dazeClient.Dial(ctx, "tcp", "127.0.0.1:65535")
	if err == nil {
		t.FailNow()
	}
	if err.Error() != "daze: dial timeout" {
		t.FailNow()
	}
}

func TestProtocalCrowTCPServerDialFailed(t *testing.T) {
	defer time.Sleep(time.Second)

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
	ctx := &daze.Context{Cid: "00000000"}
	_, err := dazeClient.Dial(ctx, "tcp", "127.0.0.1:65535")
	if err == nil {
		t.FailNow()
	}
}

func TestProtocolCrowUDP(t *testing.T) {
	defer time.Sleep(time.Second)
	echoAddr := doa.Try(net.ResolveUDPAddr("udp", EchoServerListenOn))
	echoServer := doa.Try(net.ListenUDP("udp", echoAddr))
	defer echoServer.Close()
	go func() {
		b := make([]byte, 2048)
		for {
			n, addr, err := echoServer.ReadFromUDP(b)
			if err != nil {
				break
			}
			m := doa.Try(echoServer.WriteToUDP(b[:n], addr))
			doa.Doa(n == m)
		}
	}()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
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
