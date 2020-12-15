package daze

import (
	"bytes"
	"strings"
	"testing"
)

func TestRouterRight(t *testing.T) {
	r := NewRouterRight(RoadRemote)
	if r.Road("127.0.0.1") != RoadRemote {
		t.FailNow()
	}
	if r.Road("ip.cn") != RoadRemote {
		t.FailNow()
	}
}

func TestRouterLocal(t *testing.T) {
	r := NewRouterLocal()
	if r.Road("127.0.0.1") != RoadLocale {
		t.FailNow()
	}
	if r.Road("ip.cn") != RoadPuzzle {
		t.FailNow()
	}
	if r.Road("::1") != RoadLocale {
		t.FailNow()
	}
}

func TestRouterClump(t *testing.T) {
	r := NewRouterClump(NewRouterLocal(), NewRouterRight(RoadRemote))
	if r.Road("127.0.0.1") != RoadLocale {
		t.FailNow()
	}
	if r.Road("ip.cn") != RoadRemote {
		t.FailNow()
	}
}

func TestRouterCache(t *testing.T) {
	r := NewRouterCache(NewRouterLocal())
	if r.Road("127.0.0.1") != RoadLocale {
		t.FailNow()
	}
	if r.Road("ip.cn") != RoadPuzzle {
		t.FailNow()
	}
	if a, b := r.Box.Get("127.0.0.1"); !b || a.(Road) != RoadLocale {
		t.FailNow()
	}
	if a, b := r.Box.Get("ip.cn"); !b || a.(Road) != RoadPuzzle {
		t.FailNow()
	}
	if r.Road("127.0.0.1") != RoadLocale {
		t.FailNow()
	}
	if r.Road("ip.cn") != RoadPuzzle {
		t.FailNow()
	}
}

func TestRule(t *testing.T) {
	data := strings.Join([]string{
		"R a.com *.a.com",
		"B b.com *.b.com",
		"L c.com *.c.com",
	}, "\n")
	r := NewRouterRules()
	r.FromReader(bytes.NewReader([]byte(data)))
	if r.Road("a.com") != RoadRemote {
		t.FailNow()
	}
	if r.Road("b.com") != RoadFucked {
		t.FailNow()
	}
	if r.Road("c.com") != RoadLocale {
		t.FailNow()
	}
	if r.Road("d.com") != RoadPuzzle {
		t.FailNow()
	}
	if r.Road("a.a.com") != RoadRemote {
		t.FailNow()
	}
	if r.Road("a.b.com") != RoadFucked {
		t.FailNow()
	}
	if r.Road("a.c.com") != RoadLocale {
		t.FailNow()
	}
	if r.Road("a.d.com") != RoadPuzzle {
		t.FailNow()
	}
}
