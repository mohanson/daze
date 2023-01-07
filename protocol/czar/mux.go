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
	idp chan uint8
	idx uint8
	mux *Mux
	rch chan []byte
	rer error
	rdn chan struct{}
	ron sync.Once
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
		// Errors can be safely ignored.
		s.mux.Write([]byte{s.idx, 0x02, 0x00, 0x00})
		s.idp <- s.idx
	})
	return nil
}

// Read implements io.Reader.
func (s *Stream) Read(p []byte) (int, error) {
	select {
	case b := <-s.rch:
		doa.Doa(len(p) >= len(b))
		copy(p, b)
		return len(b), nil
	case <-s.rdn:
		return 0, s.rer
	case <-s.mux.done:
		return 0, s.mux.rerr
	}
}

// A low level function, the length of the data to be written is always less than packet body size.
func (s *Stream) write(p []byte) (int, error) {
	select {
	case <-s.wdn:
		return 0, s.wer
	default:
	}
	doa.Doa(len(p) <= 2044)
	buf := make([]byte, 4+len(p))
	buf[0] = s.idx
	buf[1] = 0x01
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(p)))
	copy(buf[4:], p)
	_, err := s.mux.Write(buf)
	if err != nil {
		s.won.Do(func() {
			s.wer = err
			close(s.wdn)
		})
		// In the czar protocol, data is sent as packet, if a packet is not fully sent, it is considered unsent.
		return 0, err
	}
	return len(p), err
}

// Write implements io.Writer.
func (s *Stream) Write(p []byte) (int, error) {
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
func NewStream(idx uint8, mux *Mux) *Stream {
	return &Stream{
		idp: nil,
		idx: idx,
		mux: mux,
		rch: make(chan []byte, 32),
		rer: nil,
		rdn: make(chan struct{}),
		ron: sync.Once{},
		wer: nil,
		wdn: make(chan struct{}),
		won: sync.Once{},
	}
}

// Mux implemented the czar mux protocol.
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
