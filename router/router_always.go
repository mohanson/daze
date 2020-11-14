package router

// RouterAlways always choose the same road.
type RouterAlways struct {
	Road Road
}

// Choose.
func (r *RouterAlways) Choose(host string) Road {
	return r.Road
}

// NewRouterAlways returns a new RouterAlways.
func NewRouterAlways(road Road) *RouterAlways {
	return &RouterAlways{Road: road}
}
