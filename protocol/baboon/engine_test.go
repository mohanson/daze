package baboon

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mohanson/daze"
)

const (
	EchoServerListenOn = "127.0.0.1:28080"
	DazeServerListenOn = "127.0.0.1:21081"
	Password           = "password"
)

func TestProtocolBaboonTCP(t *testing.T) {
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

func TestProtocolBaboonUDP(t *testing.T) {
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

func TestProtocolBaboonMasker(t *testing.T) {
	log.SetOutput(ioutil.Discard)

	dazeServer := NewServer(DazeServerListenOn, Password)
	go dazeServer.Run()

	time.Sleep(time.Second)

	resp, _ := http.Get("http://" + DazeServerListenOn)
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != 200 {
		t.FailNow()
	}
	if len(body) == 0 {
		t.FailNow()
	}
}
