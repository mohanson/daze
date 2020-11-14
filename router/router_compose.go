package router

// RouterCompose concat multiple routers in series.
type RouterCompose struct {
	Member []Router
}

// Choose.
func (r *RouterCompose) Choose(host string) Road {
	var a Road
	for _, e := range r.Member {
		a = e.Choose(host)
		if a != Puzzle {
			return a
		}
	}
	return a
}

// Join a new router.
func (r *RouterCompose) Join(a Router) {
	r.Member = append(r.Member, a)
}

// NewRouterCompose returns a new RouterJoin.
func NewRouterCompose() *RouterCompose {
	return &RouterCompose{
		Member: []Router{},
	}
}
