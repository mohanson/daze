package czar

import (
	"testing"
)

func TestPriority(t *testing.T) {
	pri := NewPriority()
	pri.H(func() error {
		return nil
	})
	pri.M(func() error {
		return nil
	})
	pri.L(func() error {
		return nil
	})
}
