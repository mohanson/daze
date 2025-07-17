package rate

import (
	"math/rand/v2"
	"testing"
	"time"
)

func TestMain(t *testing.T) {
	rate := NewLimits(128, time.Millisecond)
	for range 1024 {
		maxn := rate.capacity * 4
		rate.Wait(rand.Uint64() % maxn)
	}
}
