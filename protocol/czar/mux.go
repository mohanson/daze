package czar

import (
	"encoding/binary"
	"io"
	"net"
	"sync"

	"github.com/mohanson/daze/lib/doa"
)

// A Stream managed by the multiplexer.
type Stream struct {
	idp *Sip
	idx uint8
	mux *Mux
	rbf []byte
	rch chan []byte
	rer *Err
	wer *Err
	wmu sync.Mutex
	zo0 sync.Once
	zo1 sync.Once
}

// Close implements io.Closer.
func (s *Stream) Close() error {
	s.rer.Put(io.ErrClosedPipe)
	s.wer.Put(io.ErrClosedPipe)
	s.zo0.Do(func() {
		s.wmu.Lock()
		s.mux.Write(0, []byte{s.idx, 0x02, 0x00, 0x00})
		s.wmu.Unlock()
	})
	return nil
}

// Esolc closing a stream passively.
func (s *Stream) Esolc() error {
	s.rer.Put(io.EOF)
	s.wer.Put(io.ErrClosedPipe)
	s.zo0.Do(func() {
		s.wmu.Lock()
		s.mux.Write(0, []byte{s.idx, 0x02, 0x01, 0x00})
		s.wmu.Unlock()
	})
	s.zo1.Do(func() {
		s.idp.Put(s.idx)
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
	if err := s.rer.Get(); err != nil {
		return 0, err
	}
	select {
	case s.rbf = <-s.rch:
		n := copy(p, s.rbf)
		s.rbf = s.rbf[n:]
		return n, nil
	case <-s.rer.Sig():
		return 0, s.rer.Get()
	case <-s.mux.rer.Sig():
		s.rer.Put(s.mux.rer.Get())
		return 0, s.mux.rer.Get()
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
		s.wmu.Lock()
		if err := s.wer.Get(); err != nil {
			s.wmu.Unlock()
			return n, err
		}
		_, err := s.mux.Write(1, b[:4+l])
		if err != nil {
			s.wer.Put(err)
			s.wmu.Unlock()
			return n, err
		}
		s.wmu.Unlock()
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
		rer: NewErr(),
		wer: NewErr(),
		wmu: sync.Mutex{},
		zo0: sync.Once{},
		zo1: sync.Once{},
	}
}

// Mux is used to wrap a reliable ordered connection and to multiplex it into multiple streams.
type Mux struct {
	ach chan *Stream
	con net.Conn
	idp *Sip
	rer *Err
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
	idx, err := m.idp.Get()
	if err != nil {
		return nil, err
	}
	cnt, err := m.Write(0, []byte{idx, 0x00, 0x00, 0x00})
	if err != nil {
		m.idp.Put(idx)
		return nil, err
	}
	doa.Doa(cnt == 4)
	stm := NewStream(idx, m)
	stm.idp = m.idp
	m.usb[idx] = stm
	return stm, nil
}

// Recv continues to receive data until a fatal error is encountered.
func (m *Mux) Recv() {
	var (
		bsz uint16
		buf = make([]byte, 4)
		cmd uint8
		err error
		idx uint8
		msg []byte
		old *Stream
		stm *Stream
	)
	for {
		_, err = io.ReadFull(m.con, buf[:4])
		if err != nil {
			m.rer.Put(err)
			break
		}
		idx = buf[0]
		cmd = buf[1]
		switch {
		case cmd == 0x00:
			// Make sure the stream has been closed properly.
			old = m.usb[idx]
			if old.rer.Get() == nil || old.wer.Get() == nil {
				m.con.Close()
				break
			}
			stm = NewStream(idx, m)
			// The mux server does not need to using an id pool.
			stm.idp = m.idp
			stm.idp.Set(idx)
			m.usb[idx] = stm
			m.ach <- stm
		case cmd == 0x01:
			bsz = binary.BigEndian.Uint16(buf[2:4])
			msg = make([]byte, bsz)
			_, err = io.ReadFull(m.con, msg)
			if err != nil {
				m.con.Close()
				break
			}
			stm = m.usb[idx]
			if stm.rer.Get() != nil {
				break
			}
			select {
			case stm.rch <- msg:
			case <-stm.rer.Sig():
			}
		case cmd == 0x02:
			stm = m.usb[idx]
			stm.Esolc()
		case cmd >= 0x03:
			// Packet format error, connection closed.
			m.con.Close()
		}
	}
	close(m.ach)
}

// Write writes data to the connection. The code implements a simple priority write using two locks.
func (m *Mux) Write(priority int, b []byte) (int, error) {
	if priority >= 1 {
		m.wm1.Lock()
		defer m.wm1.Unlock()
	}
	if priority >= 0 {
		m.wm0.Lock()
		defer m.wm0.Unlock()
	}
	n, err := m.con.Write(b)
	return n, err
}

// NewMux returns a new Mux.
func NewMux(conn net.Conn) *Mux {
	mux := &Mux{
		ach: make(chan *Stream),
		con: conn,
		idp: nil,
		rer: NewErr(),
		usb: make([]*Stream, 256),
		wm0: sync.Mutex{},
		wm1: sync.Mutex{},
	}
	return mux
}

// NewMuxServer returns a new MuxServer.
func NewMuxServer(conn net.Conn) *Mux {
	mux := NewMux(conn)
	mux.idp = NewSip()
	for i := range 256 {
		old := NewStream(uint8(i), mux)
		old.zo0.Do(func() {})
		old.zo1.Do(func() {})
		old.Close()
		mux.usb[i] = old
	}
	go mux.Recv()
	return mux
}

// NewMuxClient returns a new MuxClient.
func NewMuxClient(conn net.Conn) *Mux {
	mux := NewMux(conn)
	mux.idp = NewSip()
	go mux.Recv()
	return mux
}
