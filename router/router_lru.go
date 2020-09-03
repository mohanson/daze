package router

import (
	"github.com/mohanson/lru"
)

// RouterLRU cache routing results for next use.
type RouterLRU struct {
	i Router
	c *lru.Cache
}

// Choose.
func (r *RouterLRU) Choose(host string) Road {
	if a, b := r.c.Get(host); b {
		return a.(Road)
	}
	a := r.i.Choose(host)
	if a != Puzzle {
		r.c.Set(host, a)
	}
	return a
}

// NewRouterLRU returns a new RouterLRU.
func NewRouterLRU(r Router, size int) *RouterLRU {
	return &RouterLRU{
		i: r,
		c: lru.New(size),
	}
}
