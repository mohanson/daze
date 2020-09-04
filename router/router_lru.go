package router

import (
	"github.com/mohanson/lru"
)

// RouterLRU cache routing results for next use.
type RouterLRU struct {
	Pit Router
	Box *lru.Cache
}

// Choose.
func (r *RouterLRU) Choose(host string) Road {
	if a, b := r.Box.Get(host); b {
		return a.(Road)
	}
	a := r.Pit.Choose(host)
	if a != Puzzle {
		r.Box.Set(host, a)
	}
	return a
}

// NewRouterLRU returns a new RouterLRU.
func NewRouterLRU(r Router) *RouterLRU {
	return &RouterLRU{
		Pit: r,
		Box: lru.New(1024),
	}
}
