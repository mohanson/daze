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

func TestProtocolAsheTCP(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	echoListener, _ := net.Listen("tcp", "127.0.0.1:21007")
	defer echoListener.Close()
	go func() {
		c, _ := echoListener.Accept()
		io.Copy(c, c)
		c.Close()
	}()

	dazeServer := NewServer("127.0.0.1:21081", "password")
	go dazeServer.Run()

	time.Sleep(time.Second)

	dazeClient := NewClient("127.0.0.1:21081", "password")
	ctx := context.WithValue(context.Background(), "cid", "00000000")
	c, _ := dazeClient.Dial(ctx, "tcp", "127.0.0.1:21007")
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

	echoAddr, _ := net.ResolveUDPAddr("udp", "127.0.0.1:21007")
	echoServer, _ := net.ListenUDP("udp", echoAddr)
	defer echoServer.Close()

	go func() {
		for {
			b := make([]byte, 12)
			n, addr, _ := echoServer.ReadFromUDP(b)
			echoServer.WriteToUDP(b[:n], addr)
		}
	}()

	dazeServer := NewServer("127.0.0.1:21081", "password")
	go dazeServer.Run()

	time.Sleep(time.Second)

	dazeClient := NewClient("127.0.0.1:21081", "password")
	ctx := context.WithValue(context.Background(), "cid", "00000000")
	c, _ := dazeClient.Dial(ctx, "udp", "127.0.0.1:21007")
	defer c.Close()

	buf0 := []byte("Hello World!")
	c.Write(buf0)
	buf1 := make([]byte, 12)
	io.ReadFull(c, buf1)
	if !bytes.Equal(buf0, buf1) {
		t.FailNow()
	}
}
