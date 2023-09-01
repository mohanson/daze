package czar

import (
	"errors"
	"testing"

	"github.com/mohanson/daze/lib/doa"
)

func TestErr(t *testing.T) {
	er0 := errors.New("0")
	er1 := errors.New("1")
	e := Err{}
	e.Put(er0)
	e.Put(er1)
	doa.Doa(e.Get() == er0)
}
