package ashe

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"net"
	"testing"
	"time"
)

const (
	EchoServerListenOn = "127.0.0.1:28080"
	DazeServerListenOn = "127.0.0.1:21081"
	Password           = "password"
)

func TestProtocolAsheTCP(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	echoListener, _ := net.Listen("tcp", EchoServerListenOn)
	defer echoListener.Close()
	go func() {
		c, _ := echoListener.Accept()
		io.Copy(c, c)
		c.Close()
	}()

	dazeServer := NewServer(DazeServerListenOn, Password)
	go dazeServer.Run()

	time.Sleep(time.Second)

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := context.WithValue(context.Background(), "cid", "00000000")
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
	log.SetOutput(ioutil.Discard)

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
	go dazeServer.Run()

	time.Sleep(time.Second)

	dazeClient := NewClient(DazeServerListenOn, Password)
	ctx := context.WithValue(context.Background(), "cid", "00000000")
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
