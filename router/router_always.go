package router

import (
	"net"
)

// NewRouterAlways always choose the same road.
func NewRouterAlways(road Road) *RouterIPNet {
	return NewRouterIPNet([]*net.IPNet{}, Puzzle, road)
}
