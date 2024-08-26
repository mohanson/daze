package czar

import (
	"sync"
)

// Priority implement a lock with three priorities.
type Priority struct {
	l *sync.Mutex
	m *sync.Mutex
	h *sync.Mutex
}

// H executes function f with 0 priority.
func (p *Priority) H(f func() error) error {
	p.h.Lock()
	defer p.h.Unlock()
	return f()
}

// H executes function f with 1 priority.
func (p *Priority) M(f func() error) error {
	p.m.Lock()
	defer p.m.Unlock()
	p.h.Lock()
	defer p.h.Unlock()
	return f()
}

// H executes function f with 2 priority.
func (p *Priority) L(f func() error) error {
	p.l.Lock()
	defer p.l.Unlock()
	p.m.Lock()
	defer p.m.Unlock()
	p.h.Lock()
	defer p.h.Unlock()
	return f()
}

// NewPriority returns a new Priority.
func NewPriority() *Priority {
	return &Priority{
		l: &sync.Mutex{},
		m: &sync.Mutex{},
		h: &sync.Mutex{},
	}
}
