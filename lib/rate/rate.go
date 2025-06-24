package rate

import (
	"math"
	"sync"
	"time"

	"github.com/mohanson/daze/lib/doa"
)

// Limits represents a rate limiter that controls resource allocation over time.
type Limits struct {
	addition uint64
	capacity uint64
	last     time.Time
	loop     uint64
	mu       sync.Mutex
	size     uint64
	step     time.Duration
}

// Wait ensures there are enough resources (n) available, blocking if necessary.
func (l *Limits) Wait(n uint64) {
	doa.Doa(n < math.MaxUint64/2)
	doa.Doa(l.size <= l.capacity)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.loop = uint64(time.Since(l.last) / l.step)
	if l.loop > 0 {
		l.last = l.last.Add(l.step * time.Duration(l.loop))
		doa.Doa(l.loop <= math.MaxUint64/l.addition)
		doa.Doa(l.size <= math.MaxUint64-l.addition*l.loop)
		l.size = l.size + l.addition*l.loop
		l.size = min(l.size, l.capacity)
	}
	if l.size < n {
		l.loop = (n - l.size + l.addition - 1) / l.addition
		time.Sleep(l.step * time.Duration(l.loop))
		l.last = l.last.Add(l.step * time.Duration(l.loop))
		l.size = l.size + l.addition*l.loop
	}
	l.size -= n
}

// NewLimits creates a new rate limiter with rate r over period p.
func NewLimits(r uint64, p time.Duration) *Limits {
	doa.Doa(r > 0)
	doa.Doa(r < math.MaxUint64/2)
	doa.Doa(p > 0)
	g := func(a, b uint64) uint64 {
		t := uint64(0)
		for b != 0 {
			t = b
			b = a % b
			a = t
		}
		return a
	}(r, uint64(p))
	a := r / g
	s := p / time.Duration(g)
	return &Limits{
		addition: a,
		capacity: r,
		last:     time.Now(),
		mu:       sync.Mutex{},
		size:     r,
		step:     s,
	}
}
