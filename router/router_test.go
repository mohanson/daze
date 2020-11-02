package router

import (
	"bytes"
	"testing"
)

func TestAlways(t *testing.T) {
	r := NewRouterAlways(Daze)
	if r.Choose("127.0.0.1") != Daze {
		t.FailNow()
	}
	if r.Choose("ip.cn") != Daze {
		t.FailNow()
	}
}

func TestReservedIPv4(t *testing.T) {
	r := NewRouterReservedIPv4()
	if r.Choose("127.0.0.1") != Direct {
		t.FailNow()
	}
	if r.Choose("ip.cn") != Puzzle {
		t.FailNow()
	}
}

func TestReservedIPv6(t *testing.T) {
	r := NewRouterReservedIPv6()
	if r.Choose("::1") != Direct {
		t.FailNow()
	}
}

func TestCompose(t *testing.T) {
	r := NewRouterCompose()
	r.Join(NewRouterReservedIPv4())
	r.Join(NewRouterAlways(Daze))
	if r.Choose("127.0.0.1") != Direct {
		t.FailNow()
	}
	if r.Choose("ip.cn") != Daze {
		t.FailNow()
	}
}

func TestLru(t *testing.T) {
	r := NewRouterLRU(NewRouterReservedIPv4())
	if r.Choose("127.0.0.1") != Direct {
		t.FailNow()
	}
	if r.Choose("ip.cn") != Puzzle {
		t.FailNow()
	}
	if a, b := r.Box.Get("127.0.0.1"); !b || a.(Road) != Direct {
		t.FailNow()
	}
	if _, b := r.Box.Get("ip.cn"); b {
		t.FailNow()
	}
	if r.Choose("127.0.0.1") != Direct {
		t.FailNow()
	}
	if r.Choose("ip.cn") != Puzzle {
		t.FailNow()
	}
}

func TestRule(t *testing.T) {
	data := `R a.com *.a.com
B b.com *.b.com
L c.com *.c.com
`
	r := NewRouterRule()
	r.FromReader(bytes.NewReader([]byte(data)))
	if r.Choose("a.com") != Daze {
		t.FailNow()
	}
	if r.Choose("b.com") != Fucked {
		t.FailNow()
	}
	if r.Choose("c.com") != Direct {
		t.FailNow()
	}
	if r.Choose("d.com") != Puzzle {
		t.FailNow()
	}

	if r.Choose("a.a.com") != Daze {
		t.FailNow()
	}
	if r.Choose("a.b.com") != Fucked {
		t.FailNow()
	}
	if r.Choose("a.c.com") != Direct {
		t.FailNow()
	}
	if r.Choose("a.d.com") != Puzzle {
		t.FailNow()
	}
}
