package czar

import (
	"errors"
	"io"
	"log"
	"net"
	"testing"

	"github.com/godump/doa"
	"github.com/mohanson/daze"
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
	doa.Try(io.ReadFull(cli, buf[:132]))
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
					log.Println(err)
				}
				break
			}
			mux := NewMuxServer(cli)
			go func(mux *Mux) {
				for cli := range mux.Accept {
					go t.TCPServe(cli)
				}
			}(mux)
		}
	}()
	return nil
}
