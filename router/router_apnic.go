package router

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"net"
	"strconv"
	"strings"
)

// RouterApnic is a router for apnic.
type RouterApnic struct {
	Blocks []*net.IPNet
}

// Choose.
func (r *RouterApnic) Choose(host string) Road {
	l, err := net.LookupIP(host)
	if err != nil {
		return Puzzle
	}
	a := l[0]
	for _, e := range r.Blocks {
		if e.Contains(a) {
			return Direct
		}
	}
	return Puzzle
}

// Load a RULE file from reader.
func (r *RouterApnic) FromReader(f io.Reader) error {
	l := []*net.IPNet{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "apnic|CN|ipv4"):
			seps := strings.Split(line, "|")
			sep4, err := strconv.Atoi(seps[4])
			if err != nil {
				panic(err)
			}
			mask := 32 - int(math.Log2(float64(sep4)))
			_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%d", seps[3], mask))
			if err != nil {
				panic(err)
			}
			l = append(l, cidr)
		case strings.HasPrefix(line, "apnic|CN|ipv6"):
			seps := strings.Split(line, "|")
			sep4 := seps[4]
			_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%s", seps[3], sep4))
			if err != nil {
				panic(err)
			}
			l = append(l, cidr)
		}
	}
	r.Blocks = l
	return s.Err()
}

// NewRouterApnic returns a new RouterApnic.
func NewRouterApnic() *RouterApnic {
	return &RouterApnic{
		Blocks: []*net.IPNet{},
	}
}
