package czar

import (
	"errors"
	"io"
	"log"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
)

func TestProtocolMux(t *testing.T) {
	remote := Tester{daze.NewTester(EchoServerListenOn)}
	remote.Mux()
	defer remote.Close()

	mux := NewMuxClient(doa.Try(net.Dial("tcp", EchoServerListenOn)))
	defer mux.Close()
	cli := doa.Try(mux.Open())
	defer cli.Close()

	buf := make([]byte, 2048)
	doa.Try(cli.Write([]byte{0x00, 0x00, 0x00, 0x80}))
	doa.Doa(doa.Try(io.ReadFull(cli, buf[:132])) == 132)
	doa.Try(cli.Write([]byte{0x00, 0x00, 0x00, 0x80}))
	doa.Doa(doa.Try(io.ReadFull(cli, buf[:66])) == 66)
	doa.Doa(doa.Try(io.ReadFull(cli, buf[:66])) == 66)
}

func TestProtocolMuxStreamClientClose(t *testing.T) {
	remote := Tester{daze.NewTester(EchoServerListenOn)}
	remote.Mux()
	defer remote.Close()

	mux := NewMuxClient(doa.Try(net.Dial("tcp", EchoServerListenOn)))
	defer mux.Close()
	cli := doa.Try(mux.Open())
	cli.Close()
	doa.Doa(doa.Err(cli.Write([]byte{0x00, 0x00, 0x00, 0x80})) == io.ErrClosedPipe)
}

func TestProtocolMuxStreamServerClose(t *testing.T) {
	remote := Tester{daze.NewTester(EchoServerListenOn)}
	remote.Mux()
	defer remote.Close()

	mux := NewMuxClient(doa.Try(net.Dial("tcp", EchoServerListenOn)))
	defer mux.Close()
	cli := doa.Try(mux.Open())
	defer cli.Close()

	buf := make([]byte, 2048)
	doa.Try(cli.Write([]byte{0x01, 0x00, 0x00, 0x80}))
	doa.Doa(doa.Err(io.ReadFull(cli, buf[:1])) == io.EOF)
}

func TestProtocolMuxClientClose(t *testing.T) {
	remote := Tester{daze.NewTester(EchoServerListenOn)}
	remote.Mux()
	defer remote.Close()

	mux := NewMuxClient(doa.Try(net.Dial("tcp", EchoServerListenOn)))
	defer mux.Close()
	cli := doa.Try(mux.Open())
	defer cli.Close()

	mux.con.Close()
	buf := make([]byte, 2048)
	er0 := doa.Err(mux.Open())
	doa.Doa(strings.Contains(er0.Error(), "use of closed network connection"))
	er1 := doa.Err(io.ReadFull(cli, buf[:1]))
	doa.Doa(strings.Contains(er1.Error(), "use of closed network connection"))
	er2 := doa.Err(cli.Write([]byte{0x00, 0x00, 0x00, 0x80}))
	doa.Doa(strings.Contains(er2.Error(), "use of closed network connection"))
}

func TestProtocolMuxServerRecvEvilPacket(t *testing.T) {
	remote := Tester{daze.NewTester(EchoServerListenOn)}
	remote.Mux()
	defer remote.Close()

	buf := make([]byte, 2048)

	cl0 := doa.Try(net.Dial("tcp", EchoServerListenOn))
	defer cl0.Close()
	cl0.Write([]byte{0x00, 0x01, 0xff, 0xf0})
	_, er0 := io.ReadFull(cl0, buf[:1])
	doa.Doa(er0 == io.EOF)

	cl1 := doa.Try(net.Dial("tcp", EchoServerListenOn))
	defer cl1.Close()
	cl1.Write([]byte{0x00, 0x00, 0x00, 0x00})
	cl1.Write([]byte{0x00, 0x00, 0x00, 0x00})
	cl1.Write([]byte{0x00, 0x01, 0x00, 0x04, 0x00, 0x00, 0x00, 0x04})
	_, er1 := io.ReadFull(cl1, buf[:12])
	doa.Nil(er1)
	doa.Doa(slices.Equal(buf[:8], []byte{0x00, 0x01, 0x00, 0x08, 0x01, 0x00, 0x00, 0x04}))
}

type Tester struct {
	*daze.Tester
}

func (t *Tester) Mux() error {
	s, err := net.Listen("tcp", t.Listen)
	if err != nil {
		return err
	}
	t.Closer = s
	go func() {
		for {
			cli, err := s.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Println("main:", err)
				}
				break
			}
			mux := NewMuxServer(cli)
			go func(mux *Mux) {
				for cli := range mux.Accept() {
					go t.TCPServe(cli)
				}
			}(mux)
		}
	}()
	return nil
}
