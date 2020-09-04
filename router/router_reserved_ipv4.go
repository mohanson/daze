package router

import (
	"encoding/hex"
	"net"
)

// Returns a new RouterReservedIPv4 struct.
//
// Introduction:
//   See https://en.wikipedia.org/wiki/Reserved_IP_addresses
func NewRouterReservedIPv4() *RouterIPNet {
	r := []*net.IPNet{}
	for _, entry := range [][2]string{
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
	} {
		i, _ := hex.DecodeString(entry[0])
		m, _ := hex.DecodeString(entry[1])
		r = append(r, &net.IPNet{IP: i, Mask: m})
	}
	return NewRouterIPNet(r, Direct, Puzzle)
}
