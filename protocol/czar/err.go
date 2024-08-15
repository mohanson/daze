package czar

import (
	"sync"
)

// Err is an object that will only store an error once.
type Err struct {
	mux sync.Mutex // Guards following
	err error
	sig chan struct{}
}

// Get an error from Err.
func (e *Err) Get() error {
	e.mux.Lock()
	defer e.mux.Unlock()
	return e.err
}

// Put an error into Err.
func (e *Err) Put(err error) {
	e.mux.Lock()
	defer e.mux.Unlock()
	if e.err != nil {
		return
	}
	e.err = err
	close(e.sig)
}

// When any error puts, the sig will be sent.
func (e *Err) Sig() <-chan struct{} {
	return e.sig
}

func NewErr() *Err {
	return &Err{
		mux: sync.Mutex{},
		err: nil,
		sig: make(chan struct{}),
	}
}
