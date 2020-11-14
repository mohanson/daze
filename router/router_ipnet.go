package router

import (
	"net"
)

// RouterIPNet is a router by IPNets.
type RouterIPNet struct {
	Blocks []*net.IPNet
	Y      Road
	N      Road
}

// Choose.
func (r *RouterIPNet) Choose(host string) Road {
	l, err := net.LookupIP(host)
	if err != nil {
		return Puzzle
	}
	a := l[0]
	for _, e := range r.Blocks {
		if e.Contains(a) {
			return r.Y
		}
	}
	return r.N
}

// NewRouterIPNet returns a new RouterIPNet struct.
func NewRouterIPNet(blocks []*net.IPNet, y Road, n Road) *RouterIPNet {
	return &RouterIPNet{
		Blocks: blocks,
		Y:      y,
		N:      n,
	}
}
