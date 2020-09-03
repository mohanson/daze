package router

import (
	"encoding/hex"
	"net"
)

// See https://en.wikipedia.org/wiki/Reserved_IP_addresses.
var ReservedIPv6 = [][2]string{
	{"00000000000000000000000000000000", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"},
	{"00000000000000000000000000000001", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"},
	{"01000000000000000000000000000000", "FFFFFFFFFFFFFFFF0000000000000000"},
	{"0064FF9B000000000000000000000000", "FFFFFFFFFFFFFFFFFFFFFFFF00000000"},
	{"20010000000000000000000000000000", "FFFFFFFF000000000000000000000000"},
	{"20010010000000000000000000000000", "FFFFFFF0000000000000000000000000"},
	{"20010020000000000000000000000000", "FFFFFFF0000000000000000000000000"},
	{"20010DB8000000000000000000000000", "FFFFFFFF000000000000000000000000"},
	{"20020000000000000000000000000000", "FFFF0000000000000000000000000000"},
	{"FC000000000000000000000000000000", "FE000000000000000000000000000000"},
	{"FE800000000000000000000000000000", "FFC00000000000000000000000000000"},
	{"FF000000000000000000000000000000", "FF000000000000000000000000000000"},
}

// RouterReservedIPv6 is a router for reserved IPv6 address.
type RouterReservedIPv6 struct {
	Blocks []*net.IPNet
}

// Choose.
func (r *RouterReservedIPv6) Choose(host string) Road {
	l, err := net.LookupIP(host)
	if err != nil {
		return Puzzle
	}
	a := l[0]
	if len(a) != net.IPv6len {
		return Puzzle
	}
	for _, e := range r.Blocks {
		if e.Contains(a) {
			return Direct
		}
	}
	return Puzzle
}

// Returns a new RouterReservedIPv6 struct.
func NewRouterReservedIPv6() *RouterReservedIPv6 {
	blocks := []*net.IPNet{}
	for _, e := range ReservedIPv6 {
		i, _ := hex.DecodeString(e[0])
		m, _ := hex.DecodeString(e[1])
		blocks = append(blocks, &net.IPNet{IP: i, Mask: m})
	}
	return &RouterReservedIPv6{
		Blocks: blocks,
	}
}
