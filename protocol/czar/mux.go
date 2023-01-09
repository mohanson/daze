package czar

import (
	"encoding/binary"
	"io"
	"net"
	"sync"
)

// A Stream managed by the multiplexer.
type Stream struct {
	idp chan uint8
	idx uint8
	mux *Mux
	rbf []byte
	rch chan []byte
	rer error
	rdn chan struct{}
	ron sync.Once
	son sync.Once
	wer error
	wdn chan struct{}
	won sync.Once
}

// Close implements io.Closer.
func (s *Stream) Close() error {
	s.ron.Do(func() {
		s.rer = io.ErrClosedPipe
		close(s.rdn)
	})
	s.won.Do(func() {
		s.wer = io.ErrClosedPipe
		close(s.wdn)
	})
	s.son.Do(func() {
		s.mux.Write([]byte{s.idx, 0x02, 0x00, 0x00})
		s.idp <- s.idx
	})
	return nil
}

// Read implements io.Reader.
func (s *Stream) Read(p []byte) (int, error) {
	if len(s.rbf) != 0 {
		n := copy(p, s.rbf)
		s.rbf = s.rbf[n:]
		return n, nil
	}
	select {
	case s.rbf = <-s.rch:
		n := copy(p, s.rbf)
		s.rbf = s.rbf[n:]
		return n, nil
	default:
	}
	select {
	case s.rbf = <-s.rch:
		n := copy(p, s.rbf)
		s.rbf = s.rbf[n:]
		return n, nil
	case <-s.rdn:
		return 0, s.rer
	case <-s.mux.done:
		return 0, s.mux.rerr
	}
}

// Write implements io.Writer.
func (s *Stream) Write(p []byte) (int, error) {
	n := 0
	l := 0
	b := make([]byte, 2048)
	b[0] = s.idx
	b[1] = 0x01
	for {
		switch {
		case len(p) >= 2044:
			l = 2044
		case len(p) >= 1:
			l = len(p)
		case len(p) >= 0:
			return n, nil
		}
		binary.BigEndian.PutUint16(b[2:4], uint16(l))
		copy(b[4:], p[:l])
		p = p[l:]
		select {
		case <-s.wdn:
			return n, s.wer
		default:
			_, err := s.mux.Write(b[:4+l])
			if err != nil {
				return n, err
			}
			n += l
		}
	}
}

// NewStream returns a new Stream.
func NewStream(idx uint8, mux *Mux) *Stream {
	return &Stream{
		idp: nil,
		idx: idx,
		mux: mux,
		rbf: make([]byte, 0),
		rch: make(chan []byte, 32),
		rer: nil,
		rdn: make(chan struct{}),
		ron: sync.Once{},
		son: sync.Once{},
		wer: nil,
		wdn: make(chan struct{}),
		won: sync.Once{},
	}
}

// Mux is used to wrap a reliable ordered connection and to multiplex it into multiple streams.
type Mux struct {
	accept chan *Stream
	conn   net.Conn
	done   chan struct{}
	idpool chan uint8
	lock   sync.Mutex
	rerr   error
	stream []*Stream
}

// Accept is used to block until the next available stream is ready to be accepted.
func (m *Mux) Accept() chan *Stream {
	return m.accept
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (m *Mux) Close() error {
	return m.conn.Close()
}

// Open is used to create a new stream as a net.Conn.
func (m *Mux) Open() (*Stream, error) {
	idx := <-m.idpool
	_, err := m.Write([]byte{idx, 0x00, 0x00, 0x00})
	if err != nil {
		m.idpool <- idx
		return nil, err
	}
	stream := NewStream(idx, m)
	stream.idp = m.idpool
	m.stream[idx] = stream
	return stream, nil
}

// Spawn continues to receive data until a fatal error is encountered.
func (m *Mux) Spawn() {
	for {
		buf := make([]byte, 2048)
		_, err := io.ReadFull(m.conn, buf[:4])
		if err != nil {
			m.rerr = err
			break
		}
		idx := buf[0]
		cmd := buf[1]
		switch cmd {
		case 0x00:
			// Make sure the stream has been closed properly.
			select {
			case <-m.stream[idx].rdn:
			case <-m.stream[idx].wdn:
			default:
				panic("unreachable")
			}
			stream := NewStream(idx, m)
			// The mux server does not need to using an id pool.
			stream.idp = make(chan uint8, 1)
			m.stream[idx] = stream
			m.accept <- stream
		case 0x01:
			length := binary.BigEndian.Uint16(buf[2:4])
			end := length + 4
			_, err := io.ReadFull(m.conn, buf[4:end])
			if err != nil {
				break
			}
			stream := m.stream[idx]
			select {
			case stream.rch <- buf[4:end]:
			case <-stream.rdn:
			}
		case 0x02:
			stream := m.stream[idx]
			stream.ron.Do(func() {
				stream.rer = io.EOF
				close(stream.rdn)
			})
			stream.won.Do(func() {
				stream.wer = io.ErrClosedPipe
				close(stream.wdn)
			})
			stream.son.Do(func() {
				stream.idp <- stream.idx
			})
		}
	}
	close(m.accept)
	close(m.done)
}

// Write writes data to the connection.
func (m *Mux) Write(b []byte) (int, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.conn.Write(b)
}

// NewMux returns a new Mux.
func NewMux(conn net.Conn) *Mux {
	m := &Mux{
		accept: make(chan *Stream),
		conn:   conn,
		done:   make(chan struct{}),
		idpool: nil,
		lock:   sync.Mutex{},
		rerr:   nil,
		stream: make([]*Stream, 256),
	}
	go m.Spawn()
	return m
}

// NewMuxServer returns a new MuxServer.
func NewMuxServer(conn net.Conn) *Mux {
	mux := NewMux(conn)
	for i := 0; i < 256; i++ {
		stream := NewStream(uint8(i), mux)
		stream.ron.Do(func() {
			stream.rer = io.ErrClosedPipe
			close(stream.rdn)
		})
		stream.won.Do(func() {
			stream.wer = io.ErrClosedPipe
			close(stream.wdn)
		})
		stream.son.Do(func() {
		})
		mux.stream[i] = stream
	}
	return mux
}

// NewMuxClient returns a new MuxClient.
func NewMuxClient(conn net.Conn) *Mux {
	idp := make(chan uint8, 256)
	for i := 0; i < 256; i++ {
		idp <- uint8(i)
	}
	mux := NewMux(conn)
	mux.idpool = idp
	return mux
}
