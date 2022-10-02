package czar

import (
	"encoding/binary"
	"io"
	"net"
	"sync"

	"github.com/godump/doa"
)

// A Stream managed by the multiplexer.
type Stream struct {
	ci chan uint8
	cr chan []byte
	// Closes when the Close function is called.
	dn chan struct{}
	id uint8
	mx *Mux
	// Protects closing.
	on sync.Once
}

// Close implements io.Closer.
func (s *Stream) Close() error {
	s.on.Do(func() {
		close(s.dn)
		buf := make([]byte, 4)
		buf[0] = s.id
		buf[1] = 0x02
		select {
		case s.mx.Send <- buf:
		case <-s.mx.SendDone:
		}
		s.ci <- s.id
	})
	return nil
}

// Read implements io.Reader.
func (s *Stream) Read(p []byte) (int, error) {
	select {
	case <-s.dn:
		return 0, io.ErrClosedPipe
	default:
	}
	select {
	case b, ok := <-s.cr:
		if !ok {
			return 0, io.EOF
		}
		doa.Doa(len(p) >= len(b))
		copy(p, b)
		return len(b), nil
	case <-s.mx.RecvDone:
		return 0, io.ErrClosedPipe
	}
}

// A low level function, the length of the data to be written is always less than packet body size.
func (s *Stream) write(p []byte) (int, error) {
	doa.Doa(len(p) <= 2044)
	buf := make([]byte, 4+len(p))
	buf[0] = s.id
	buf[1] = 0x01
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(p)))
	copy(buf[4:], p)
	select {
	case s.mx.Send <- buf:
		return len(p), nil
	case <-s.mx.SendDone:
		return 0, io.ErrClosedPipe
	}
}

// Write implements io.Writer.
func (s *Stream) Write(p []byte) (int, error) {
	select {
	case <-s.dn:
		return 0, io.ErrClosedPipe
	default:
	}
	f := 0
	e := f + 2044
	n := 0
	for e < len(p) {
		m, err := s.write(p[f:e])
		n += m
		if err != nil {
			return n, err
		}
		f = e
		e = f + 2044
	}
	m, err := s.write(p[f:])
	if err != nil {
		return n, err
	}
	n += m
	doa.Doa(len(p) == n)
	return n, err
}

// NewStream returns a new Stream.
func NewStream(id uint8, mx *Mux) *Stream {
	return &Stream{
		cr: make(chan []byte, 32),
		dn: make(chan struct{}),
		id: id,
		mx: mx,
		on: sync.Once{},
	}
}

// Mux implemented the czar mux protocol.
type Mux struct {
	// Accept is used to block until the next available stream is ready to be accepted.
	Accept   chan *Stream
	Conn     net.Conn
	IDPool   chan uint8
	Recv     chan []byte
	RecvDone chan struct{}
	Send     chan []byte
	SendDone chan struct{}
	Stream   []*Stream
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (m *Mux) Close() error {
	return m.Conn.Close()
}

// Stream is used to create a new stream as a net.Conn.
func (m *Mux) Open() (*Stream, error) {
	idx := <-m.IDPool
	buf := make([]byte, 4)
	buf[0] = idx
	buf[1] = 0x00
	select {
	case m.Send <- buf:
		stream := NewStream(idx, m)
		stream.ci = m.IDPool
		m.Stream[idx] = stream
		return stream, nil
	case <-m.RecvDone:
		return nil, io.ErrClosedPipe
	case <-m.SendDone:
		return nil, io.ErrClosedPipe
	}
}

// Data processing and distribution.
func (m *Mux) give() {
	for buf := range m.Recv {
		idx := buf[0]
		cmd := buf[1]
		switch cmd {
		case 0x00:
			stream := NewStream(idx, m)
			// The mux server does not need to using an id pool.
			stream.ci = make(chan uint8, 1)
			// Make sure the stream has been closed properly.
			if m.Stream[idx] != nil {
				<-m.Stream[idx].dn
			}
			m.Stream[idx] = stream
			m.Accept <- stream
		case 0x01:
			length := binary.BigEndian.Uint16(buf[2:4])
			select {
			case m.Stream[idx].cr <- buf[4 : 4+length]:
			case <-m.Stream[idx].dn:
			}
		case 0x02:
			close(m.Stream[idx].cr)
		}
	}
	close(m.RecvDone)
	close(m.Accept)
}

// Recv loop continues to receive data until a fatal error is encountered.
func (m *Mux) recv() {
	for {
		buf := make([]byte, 2048)
		_, err := io.ReadFull(m.Conn, buf[:4])
		if err != nil {
			break
		}
		cmd := buf[1]
		if cmd == 0x01 {
			msg := binary.BigEndian.Uint16(buf[2:4])
			_, err := io.ReadFull(m.Conn, buf[4:4+msg])
			if err != nil {
				break
			}
		}
		m.Recv <- buf
	}
	close(m.Recv)
}

// Send loop is a long running goroutine that sends data.
func (m *Mux) send() {
	for data := range m.Send {
		_, err := m.Conn.Write(data)
		if err != nil {
			break
		}
	}
	close(m.SendDone)
}

// NewMux returns a new Mux.
func NewMux(conn net.Conn) *Mux {
	d := &Mux{
		Accept:   make(chan *Stream),
		Conn:     conn,
		Recv:     make(chan []byte),
		RecvDone: make(chan struct{}),
		Send:     make(chan []byte),
		SendDone: make(chan struct{}),
		Stream:   make([]*Stream, 256),
	}
	go d.give()
	go d.recv()
	go d.send()
	return d
}

// NewMuxServer returns a new MuxServer.
func NewMuxServer(conn net.Conn) *Mux {
	return NewMux(conn)
}

// NewMuxClient returns a new MuxClient.
func NewMuxClient(conn net.Conn) *Mux {
	idp := make(chan uint8, 256)
	for i := 0; i < 256; i++ {
		idp <- uint8(i)
	}
	mux := NewMux(conn)
	mux.IDPool = idp
	return mux
}
