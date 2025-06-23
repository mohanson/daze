package rate

import (
	"sync"
	"time"
)

// Limits represents a rate limiter that controls resource allocation over time.
type Limits struct {
	addition uint64
	capacity uint64
	last     time.Time
	mu       sync.Mutex
	size     uint64
	step     time.Duration
}

// Wait ensures there are enough resources (n) available, blocking if necessary.
func (l *Limits) Wait(n uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	loop := uint64(time.Since(l.last) / l.step)
	l.last = l.last.Add(l.step * time.Duration(loop))
	l.size = l.size + loop*l.addition
	l.size = min(l.size, l.capacity)
	if l.size < n {
		loop := (n - l.size + l.addition - 1) / l.addition
		time.Sleep(l.step * time.Duration(loop))
		l.last = l.last.Add(l.step * time.Duration(loop))
		l.size = l.size + loop*l.addition
	}
	l.size -= n
}

// NewLimits creates a new rate limiter with rate r over period p.
func NewLimits(r uint64, p time.Duration) *Limits {
	g := func(a, b uint64) uint64 {
		t := uint64(0)
		for b != 0 {
			t = b
			b = a % b
			a = t
		}
		return a
	}(r, uint64(p))
	r = r / g
	p = p / time.Duration(g)
	return &Limits{
		addition: r,
		capacity: r * 2,
		last:     time.Now(),
		mu:       sync.Mutex{},
		size:     r,
		step:     p,
	}
}
