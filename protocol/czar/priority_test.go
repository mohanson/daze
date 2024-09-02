package czar

import (
	"testing"
)

func TestProtocolCzarPriority(t *testing.T) {
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
