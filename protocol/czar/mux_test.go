package czar

import (
	"encoding/binary"
	"errors"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"strings"
	"testing"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
)

func TestProtocolCzarMux(t *testing.T) {
	rmt := &Tester{daze.NewTester(EchoServerListenOn)}
	rmt.Mux()
	defer rmt.Close()

	mux := NewMuxClient(doa.Try(net.Dial("tcp", EchoServerListenOn)))
	defer mux.Close()
	cli := doa.Try(mux.Open())
	defer cli.Close()

	var (
		bsz = max(4, int(rand.Uint32N(256)))
		buf = make([]byte, bsz)
		cnt int
		rsz = int(rand.Uint32N(65536))
	)

	copy(buf[0:2], []byte{0x00, 0x00})
	binary.BigEndian.PutUint16(buf[2:], uint16(rsz))
	doa.Try(cli.Write(buf[:4]))
	cnt = 0
	for {
		e := min(rand.IntN(bsz+1), rsz-cnt)
		n := doa.Try(io.ReadFull(cli, buf[:e]))
		for i := range n {
			doa.Doa(buf[i] == 0x00)
		}
		cnt += n
		if cnt == rsz {
			break
		}
	}

	copy(buf[0:2], []byte{0x01, 0x00})
	binary.BigEndian.PutUint16(buf[2:], uint16(rsz))
	doa.Try(cli.Write(buf[:4]))
	for i := range bsz {
		buf[i] = 0x00
	}
	cnt = 0
	for {
		e := min(rand.IntN(bsz+1), rsz-cnt)
		n := doa.Try(cli.Write(buf[:e]))
		cnt += n
		if cnt == rsz {
			break
		}
	}
}

func TestProtocolCzarMuxStreamClientClose(t *testing.T) {
	rmt := &Tester{daze.NewTester(EchoServerListenOn)}
	rmt.Mux()
	defer rmt.Close()

	mux := NewMuxClient(doa.Try(net.Dial("tcp", EchoServerListenOn)))
	defer mux.Close()
	cli := doa.Try(mux.Open())
	cli.Close()
	doa.Doa(doa.Err(cli.Write([]byte{0x00, 0x00, 0x00, 0x80})) == io.ErrClosedPipe)
}

func TestProtocolCzarMuxStreamServerClose(t *testing.T) {
	rmt := Tester{daze.NewTester(EchoServerListenOn)}
	rmt.Mux()
	defer rmt.Close()

	mux := NewMuxClient(doa.Try(net.Dial("tcp", EchoServerListenOn)))
	defer mux.Close()
	cli := doa.Try(mux.Open())
	defer cli.Close()

	buf := make([]byte, 2048)
	doa.Try(cli.Write([]byte{0x02, 0x00, 0x00, 0x80}))
	doa.Doa(doa.Err(io.ReadFull(cli, buf[:1])) == io.EOF)
}

func TestProtocolCzarMuxStreamClientReuse(t *testing.T) {
	rmt := &Tester{daze.NewTester(EchoServerListenOn)}
	rmt.Mux()
	defer rmt.Close()

	mux := NewMuxClient(doa.Try(net.Dial("tcp", EchoServerListenOn)))
	defer mux.Close()
	buf := make([]byte, 0x8000)

	cl0 := doa.Try(mux.Open())
	cl0.Write([]byte{0x00, 0x00, 0x80, 0x00})
	cl0.Close()
	for {
		idx := doa.Try(mux.idp.Get())
		mux.idp.Put(idx)
		if idx == 0x00 {
			break
		}
	}
	cl1 := doa.Try(mux.Open())
	doa.Doa(cl1.idx == 0x00)
	doa.Try(cl1.Write([]byte{0x00, 0x01, 0x80, 0x00}))
	doa.Doa(doa.Try(io.ReadFull(cl1, buf)) == 0x8000)
	for i := range 0x8000 {
		doa.Doa(buf[i] == 0x01)
	}
	cl1.Close()
}

func TestProtocolCzarMuxClientClose(t *testing.T) {
	rmt := &Tester{daze.NewTester(EchoServerListenOn)}
	rmt.Mux()
	defer rmt.Close()

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

func TestProtocolCzarMuxServerRecvEvilPacket(t *testing.T) {
	rmt := &Tester{daze.NewTester(EchoServerListenOn)}
	rmt.Mux()
	defer rmt.Close()

	buf := make([]byte, 2048)
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
