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
	rer Err
	rdn chan struct{}
	ron sync.Once
	son sync.Once
	wer Err
	wdn chan struct{}
	won sync.Once
}

// Close implements io.Closer.
func (s *Stream) Close() error {
	s.rer.Put(io.ErrClosedPipe)
	s.wer.Put(io.ErrClosedPipe)
	s.ron.Do(func() { close(s.rdn) })
	s.won.Do(func() { close(s.wdn) })
	s.son.Do(func() {
		s.mux.Write(0, []byte{s.idx, 0x02, 0x00, 0x00})
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
	if len(s.rch) != 0 {
		s.rbf = <-s.rch
		n := copy(p, s.rbf)
		s.rbf = s.rbf[n:]
		return n, nil
	}
	select {
	case s.rbf = <-s.rch:
		n := copy(p, s.rbf)
		s.rbf = s.rbf[n:]
		return n, nil
	case <-s.rdn:
		return 0, s.rer.err
	case <-s.mux.rdn:
		return 0, s.mux.rer
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
		if err := s.wer.Get(); err != nil {
			return n, err
		}
		_, err := s.mux.Write(1, b[:4+l])
		if err != nil {
			return n, err
		}
		n += l
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
		rer: Err{},
		rdn: make(chan struct{}),
		ron: sync.Once{},
		son: sync.Once{},
		wer: Err{},
		wdn: make(chan struct{}),
		won: sync.Once{},
	}
}

// Mux is used to wrap a reliable ordered connection and to multiplex it into multiple streams.
type Mux struct {
	ach chan *Stream
	con net.Conn
	idp chan uint8
	rdn chan struct{}
	rer error
	usb []*Stream
	wm0 sync.Mutex
	wm1 sync.Mutex
}

// Accept is used to block until the next available stream is ready to be accepted.
func (m *Mux) Accept() chan *Stream {
	return m.ach
}

// Close closes the connection.
// Any blocked Read or Write operations will be unblocked and return errors.
func (m *Mux) Close() error {
	return m.con.Close()
}

// Open is used to create a new stream as a net.Conn.
func (m *Mux) Open() (*Stream, error) {
	idx := <-m.idp
	_, err := m.Write(0, []byte{idx, 0x00, 0x00, 0x00})
	if err != nil {
		m.idp <- idx
		return nil, err
	}
	stm := NewStream(idx, m)
	stm.idp = m.idp
	m.usb[idx] = stm
	return stm, nil
}

// Spawn continues to receive data until a fatal error is encountered.
func (m *Mux) Spawn() {
	for {
		buf := make([]byte, 2048)
		_, err := io.ReadFull(m.con, buf[:4])
		if err != nil {
			m.rer = err
			break
		}
		idx := buf[0]
		cmd := buf[1]
		switch {
		case cmd == 0x00:
			// Make sure the stream has been closed properly.
			old := m.usb[idx]
			old.ron.Do(func() { close(old.rdn) })
			old.won.Do(func() { close(old.wdn) })
			old.son.Do(func() { old.idp <- old.idx })
			stm := NewStream(idx, m)
			// The mux server does not need to using an id pool.
			stm.idp = make(chan uint8, 1)
			m.usb[idx] = stm
			m.ach <- stm
		case cmd == 0x01:
			bsz := binary.BigEndian.Uint16(buf[2:4])
			if bsz > 2044 {
				// Packet format error, connection closed.
				m.con.Close()
				break
			}
			end := bsz + 4
			_, err := io.ReadFull(m.con, buf[4:end])
			if err != nil {
				break
			}
			stm := m.usb[idx]
			select {
			case stm.rch <- buf[4:end]:
			case <-stm.rdn:
			}
		case cmd == 0x02:
			stm := m.usb[idx]
			stm.rer.Put(io.EOF)
			stm.wer.Put(io.ErrClosedPipe)
			stm.ron.Do(func() { close(stm.rdn) })
			stm.won.Do(func() { close(stm.wdn) })
			stm.son.Do(func() { stm.idp <- stm.idx })
		case cmd >= 0x03:
			// Packet format error, connection closed.
			m.con.Close()
		}
	}
	close(m.ach)
	close(m.rdn)
}

// Write writes data to the connection. The code implements a simple priority write using two locks.
func (m *Mux) Write(priority int, b []byte) (int, error) {
	if priority >= 1 {
		m.wm1.Lock()
	}
	m.wm0.Lock()
	n, err := m.con.Write(b)
	m.wm0.Unlock()
	if priority >= 1 {
		m.wm1.Unlock()
	}
	return n, err
}

// NewMux returns a new Mux.
func NewMux(conn net.Conn) *Mux {
	mux := &Mux{
		ach: make(chan *Stream),
		con: conn,
		idp: nil,
		rdn: make(chan struct{}),
		rer: nil,
		usb: make([]*Stream, 256),
		wm0: sync.Mutex{},
		wm1: sync.Mutex{},
	}
	return mux
}

// NewMuxServer returns a new MuxServer.
func NewMuxServer(conn net.Conn) *Mux {
	mux := NewMux(conn)
	for i := 0; i < 256; i++ {
		stm := NewStream(uint8(i), mux)
		stm.son.Do(func() {})
		stm.Close()
		mux.usb[i] = stm
	}
	go mux.Spawn()
	return mux
}

// NewMuxClient returns a new MuxClient.
func NewMuxClient(conn net.Conn) *Mux {
	idp := make(chan uint8, 256)
	for i := 0; i < 256; i++ {
		idp <- uint8(i)
	}
	mux := NewMux(conn)
	mux.idp = idp
	go mux.Spawn()
	return mux
}
