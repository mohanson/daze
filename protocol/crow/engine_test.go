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

// func TestProtocolAsheTCP(t *testing.T) {
// 	defer time.Sleep(time.Second)

// 	dazeServer := NewServer(DazeServerListenOn, Password)
// 	defer dazeServer.Close()
// 	go dazeServer.Run()

// 	time.Sleep(time.Second)

// 	dazeClient := NewClient(DazeServerListenOn, Password)
// 	dazeClient.Run()
// 	dazeClient.Writer <- []byte{0x01, 0x00, 0x08}
// }

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
	dazeClient.Run()
	// dazeClient.Writer <- []byte{0x01, 0x00, 0x08}

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

	// cli2 := doa.Try(dazeClient.Dial(ctx, "tcp", EchoServerListenOn))
	// defer cli2.Close()

	// buf02 := []byte("Hello World!")
	// cli2.Write(buf0)
	// buf12 := make([]byte, 12)
	// io.ReadFull(cli2, buf12)
	// if !bytes.Equal(buf02, buf12) {
	// 	t.FailNow()
	// }
}
