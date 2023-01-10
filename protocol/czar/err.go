package czar

import (
	"sync"
)

// Err is an object that will only store an error once.
type Err struct {
	sync.Mutex // Guards following
	err        error
}

// Put an error into Err.
func (a *Err) Put(err error) {
	a.Lock()
	defer a.Unlock()
	if a.err != nil {
		return
	}
	a.err = err
}

// Get an error from Err.
func (a *Err) Get() error {
	a.Lock()
	defer a.Unlock()
	return a.err
}
