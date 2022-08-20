package czar

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net"
	"sync/atomic"
	"testing"

	"github.com/godump/doa"
	"github.com/mohanson/daze"
)

const (
	EchoServerListenOn = "127.0.0.1:28080"
	DazeServerListenOn = "127.0.0.1:21081"
	Password           = "password"
)

func TestProtocalCzarTCP(t *testing.T) {
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
	ctx := &daze.Context{}
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

func TestProtocalCzarTCPClientClose(t *testing.T) {
	echoListener := doa.Try(net.Listen("tcp", EchoServerListenOn))
	defer echoListener.Close()
	chanLive := make(chan uint32, 1)
	go func() {
		live := uint32(0)
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
				atomic.AddUint32(&live, 1)
				chanLive <- live
				io.Copy(c, c)
				atomic.AddUint32(&live, ^uint32(0))
				chanLive <- live
			}(c)
		}
	}()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	if <-chanLive != 1 {
		t.FailNow()
	}
	cli.Close()
	if <-chanLive != 0 {
		t.FailNow()
	}
}

func TestProtocalCzarTCPClientDialFailed(t *testing.T) {
	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
	ctx := &daze.Context{}
	_, err := dazeClient.Dial(ctx, "tcp", "127.0.0.1:65535")
	if err == nil {
		t.FailNow()
	}
	if err.Error() != "daze: dial timeout" {
		t.FailNow()
	}
}

func TestProtocalCzarTCPClientHighLoad(t *testing.T) {
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
				_, err := io.CopyN(io.Discard, c, 32*1024*1024)
				if err != nil {
					c.Write([]byte{1})
				} else {
					c.Write([]byte{0})
				}
			}(c)
		}
	}()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	buf := make([]byte, 32*1024)
	for i := 0; i < 1024; i++ {
		daze.Random.Read(buf)
		cli.Write(buf)
	}
	io.ReadFull(cli, buf[:1])
	if buf[0] != 0 {
		t.FailNow()
	}
}

func TestProtocalCzarTCPServerClose(t *testing.T) {
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
	ctx := &daze.Context{}
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

func TestProtocalCzarTCPServerDialFailed(t *testing.T) {
	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
	ctx := &daze.Context{}
	_, err := dazeClient.Dial(ctx, "tcp", "127.0.0.1:65535")
	if err == nil {
		t.FailNow()
	}
}

func TestProtocalCzarTCPServerHighLoad(t *testing.T) {
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
				buf := make([]byte, 32*1024)
				for i := 0; i < 1024; i++ {
					daze.Random.Read(buf)
					_, err := c.Write(buf)
					if err != nil {
						break
					}
				}
			}(c)
		}
	}()

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	dazeServer.Run()

	dazeClient := NewClient(DazeServerListenOn, Password)
	defer dazeClient.Close()
	ctx := &daze.Context{}
	cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	defer cli.Close()

	_, err := io.CopyN(io.Discard, cli, 32*1024*1024)
	if err != nil {
		t.FailNow()
	}
}

func TestProtocolCrowUDP(t *testing.T) {
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
	ctx := &daze.Context{}
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
