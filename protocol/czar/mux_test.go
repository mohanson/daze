package czar

import (
	"errors"
	"io"
	"log"
	"net"
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
	doa.Doa(doa.Try(io.ReadFull(cli, buf[:128])) == 128)
	doa.Try(cli.Write([]byte{0x00, 0x00, 0x00, 0x80}))
	doa.Doa(doa.Try(io.ReadFull(cli, buf[:64])) == 64)
	doa.Doa(doa.Try(io.ReadFull(cli, buf[:64])) == 64)
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
	doa.Try(cli.Write([]byte{0x02, 0x00, 0x00, 0x80}))
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

func TestProtocolMuxStreamClientReuse(t *testing.T) {
	remote := Tester{daze.NewTester(EchoServerListenOn)}
	remote.Mux()
	defer remote.Close()

	mux := NewMuxClient(doa.Try(net.Dial("tcp", EchoServerListenOn)))
	defer mux.Close()
	buf := make([]byte, 0x8000)

	cl0 := doa.Try(mux.Open())
	cl0.Write([]byte{0x00, 0x00, 0x80, 0x00})
	cl0.Close()

	cl1 := doa.Try(mux.Open())
	doa.Try(cl1.Write([]byte{0x00, 0x01, 0x80, 0x00}))
	doa.Doa(doa.Try(io.ReadFull(cl1, buf)) == 0x8000)
	for i := range 0x8000 {
		doa.Doa(buf[i] == 0x01)
	}
	cl1.Close()
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
	_, er1 := io.ReadFull(cl1, buf[:1])
	doa.Doa(er1 != nil)
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
			go func() {
				for cli := range mux.Accept() {
					go t.TCPServe(cli)
				}
			}()
		}
	}()
	return nil
}
