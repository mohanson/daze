package crow

import (
	"io"
	"testing"
	"time"
)

const (
	EchoServerListenOn = "127.0.0.1:28080"
	DazeServerListenOn = "127.0.0.1:21081"
	Password           = "password"
)

func TestProtocolAsheTCP(t *testing.T) {
	defer time.Sleep(time.Second)

	dazeServer := NewServer(DazeServerListenOn, Password)
	defer dazeServer.Close()
	go dazeServer.Run()

	time.Sleep(time.Second)

	dazeClient := NewClient(DazeServerListenOn, Password)
	dazeClient.Conn.Write([]byte{0x01, 0x08, 0x00})
	buf := make([]byte, 4096)
	io.ReadFull(dazeClient.Conn, buf[:3])
	if buf[0] != 0x00 || buf[1] != 0x08 || buf[2] != 0x00 {
		t.FailNow()
	}
	n, _ := io.ReadFull(dazeClient.Conn, buf[:2048])
	if n != 2048 {
		t.FailNow()
	}

	// ctx := &daze.Context{Cid: "00000000"}
	// cli := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	// defer cli.Close()

	// buf0 := []byte("Hello World!")
	// cli.Write(buf0)
}
