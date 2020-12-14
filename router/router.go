package router

import (
	"bufio"
	"fmt"
	"io"
	"math/bits"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mohanson/daze"
	"github.com/mohanson/doa"
)

// NewAlways always returns same road.
func NewAlways(road daze.Road) *daze.RouterIPNet {
	return &daze.RouterIPNet{L: []*net.IPNet{}, Y: road, N: road}
}

// When compounding region, return daze.RoadLocale.
// The original address of the f is http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest
func NewApnic(f io.Reader, region string) *daze.RouterIPNet {
	ipv4Prefix := fmt.Sprintf("apnic|%s|ipv4", region)
	ipv6Prefix := fmt.Sprintf("apnic|%s|ipv6", region)
	r := []*net.IPNet{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, ipv4Prefix):
			seps := strings.Split(line, "|")
			sep4 := doa.Try2(strconv.ParseUint(seps[4], 0, 32)).(uint64)
			// Determine whether it is a power of 2
			if sep4&(sep4-1) != 0 {
				panic("unreachable")
			}
			mask := bits.LeadingZeros64(sep4) - 31
			_, cidr := doa.Try3(net.ParseCIDR(fmt.Sprintf("%s/%d", seps[3], mask)))
			r = append(r, cidr.(*net.IPNet))
		case strings.HasPrefix(line, ipv6Prefix):
			seps := strings.Split(line, "|")
			sep4 := seps[4]
			_, cidr := doa.Try3(net.ParseCIDR(fmt.Sprintf("%s/%s", seps[3], sep4)))
			r = append(r, cidr.(*net.IPNet))
		}
	}
	return daze.NewRouterIPNet(r, daze.RoadLocale, daze.RoadPuzzle)
}

// A single rule in RULE file.
type Glob struct {
	Pattern string
	Road    daze.Road
}

// RULE file aims to be a minimal configuration file format that's easy to read due to obvious semantics.
// There are two parts per line on RULE file: mode and glob. mode are on the left of the space sign and glob are on the
// right. mode is an char and describes whether the host should go proxy, glob supported glob-style patterns:
//
//   h?llo matches hello, hallo and hxllo
//   h*llo matches hllo and heeeello
//   h[ae]llo matches hello and hallo, but not hillo
//   h[^e]llo matches hallo, hbllo, ... but not hello
//   h[a-b]llo matches hallo and hbllo
//
// This is a RULE document:
//   L a.com
//   R b.com
//   B c.com
//
// L(ocale)  means using locale network
// R(emote)  means using remote network
// B(anned)  means block it
type Rule struct {
	L []Glob
}

// Road implements daze.Router.
func (r *Rule) Road(host string) daze.Road {
	for _, e := range r.L {
		if doa.Try2(filepath.Match(e.Pattern, host)).(bool) {
			return e.Road
		}
	}
	return daze.RoadPuzzle
}

// Load a RULE file from reader.
func (r *Rule) FromReader(f io.Reader) error {
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		seps := strings.Split(line, " ")
		if len(seps) < 2 {
			continue
		}
		switch seps[0] {
		case "#":
		case "L":
			for _, e := range seps[1:] {
				r.L = append(r.L, Glob{Pattern: e, Road: daze.RoadLocale})
			}
		case "R":
			for _, e := range seps[1:] {
				r.L = append(r.L, Glob{Pattern: e, Road: daze.RoadRemote})
			}
		case "B":
			for _, e := range seps[1:] {
				r.L = append(r.L, Glob{Pattern: e, Road: daze.RoadFucked})
			}
		}
	}
	return scanner.Err()
}

// NewRouterRule returns a new RoaderRule.
func NewRule() *Rule {
	return &Rule{
		L: []Glob{},
	}
}
