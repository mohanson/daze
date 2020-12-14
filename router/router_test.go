package router

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mohanson/daze"
)

func TestAlways(t *testing.T) {
	r := NewAlways(daze.RoadRemote)
	if r.Road("127.0.0.1") != daze.RoadRemote {
		t.FailNow()
	}
	if r.Road("ip.cn") != daze.RoadRemote {
		t.FailNow()
	}
}

func TestReservedIPv4(t *testing.T) {
	r := daze.NewRouterReservedIP()
	if r.Road("127.0.0.1") != daze.RoadLocale {
		t.FailNow()
	}
	if r.Road("ip.cn") != daze.RoadPuzzle {
		t.FailNow()
	}
}

func TestReservedIPv6(t *testing.T) {
	r := daze.NewRouterReservedIP()
	if r.Road("::1") != daze.RoadLocale {
		t.FailNow()
	}
}

func TestClump(t *testing.T) {
	r := daze.NewRouterClump(
		daze.NewRouterReservedIP(),
		NewAlways(daze.RoadRemote),
	)
	if r.Road("127.0.0.1") != daze.RoadLocale {
		t.FailNow()
	}
	if r.Road("ip.cn") != daze.RoadRemote {
		t.FailNow()
	}
}

func TestCache(t *testing.T) {
	r := daze.NewRouterCache(daze.NewRouterReservedIP())
	if r.Road("127.0.0.1") != daze.RoadLocale {
		t.FailNow()
	}
	if r.Road("ip.cn") != daze.RoadPuzzle {
		t.FailNow()
	}
	if a, b := r.Box.Get("127.0.0.1"); !b || a.(daze.Road) != daze.RoadLocale {
		t.FailNow()
	}
	if a, b := r.Box.Get("ip.cn"); !b || a.(daze.Road) != daze.RoadPuzzle {
		t.FailNow()
	}
	if r.Road("127.0.0.1") != daze.RoadLocale {
		t.FailNow()
	}
	if r.Road("ip.cn") != daze.RoadPuzzle {
		t.FailNow()
	}
}

func TestRule(t *testing.T) {
	data := strings.Join([]string{"R a.com *.a.com", "B b.com *.b.com", "L c.com *.c.com"}, "\n")
	r := NewRule()
	r.FromReader(bytes.NewReader([]byte(data)))
	if r.Road("a.com") != daze.RoadRemote {
		t.FailNow()
	}
	if r.Road("b.com") != daze.RoadFucked {
		t.FailNow()
	}
	if r.Road("c.com") != daze.RoadLocale {
		t.FailNow()
	}
	if r.Road("d.com") != daze.RoadPuzzle {
		t.FailNow()
	}

	if r.Road("a.a.com") != daze.RoadRemote {
		t.FailNow()
	}
	if r.Road("a.b.com") != daze.RoadFucked {
		t.FailNow()
	}
	if r.Road("a.c.com") != daze.RoadLocale {
		t.FailNow()
	}
	if r.Road("a.d.com") != daze.RoadPuzzle {
		t.FailNow()
	}
}
