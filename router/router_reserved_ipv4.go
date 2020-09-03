package router

import (
	"encoding/hex"
	"net"
)

// See https://en.wikipedia.org/wiki/Reserved_IP_addresses.
var ReservedIPv4 = [][2]string{
	{"00000000", "FF000000"},
	{"0A000000", "FF000000"},
	{"7F000000", "FF000000"},
	{"A9FE0000", "FFFF0000"},
	{"AC100000", "FFF00000"},
	{"C0000000", "FFFFFFF8"},
	{"C00000AA", "FFFFFFFE"},
	{"C0000200", "FFFFFF00"},
	{"C0A80000", "FFFF0000"},
	{"C6120000", "FFFE0000"},
	{"C6336400", "FFFFFF00"},
	{"CB007100", "FFFFFF00"},
	{"F0000000", "F0000000"},
	{"FFFFFFFF", "FFFFFFFF"},
}

// RouterReservedIPv4 is a router for reserved IPv4 address.
type RouterReservedIPv4 struct {
	Blocks []*net.IPNet
}

// Choose.
func (r *RouterReservedIPv4) Choose(host string) Road {
	l, err := net.LookupIP(host)
	if err != nil {
		return Puzzle
	}
	a := l[0]
	if len(a) != net.IPv4len {
		return Puzzle
	}
	for _, e := range r.Blocks {
		if e.Contains(a) {
			return Direct
		}
	}
	return Puzzle
}

// Returns a new RouterReservedIPv4 struct.
func NewRouterReservedIPv4() *RouterReservedIPv4 {
	blocks := []*net.IPNet{}
	for _, e := range ReservedIPv4 {
		i, _ := hex.DecodeString(e[0])
		m, _ := hex.DecodeString(e[1])
		blocks = append(blocks, &net.IPNet{IP: i, Mask: m})
	}
	return &RouterReservedIPv4{
		Blocks: blocks,
	}
}
