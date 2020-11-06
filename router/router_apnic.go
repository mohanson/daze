package router

import (
	"bufio"
	"fmt"
	"io"
	"math/bits"
	"net"
	"strconv"
	"strings"

	"github.com/mohanson/doa"
)

// NewRouterApnic returns a new RouterApnic.
// Pass the file in as a stream: http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest
func NewRouterApnic(f io.Reader, region string) *RouterIPNet {
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
	return NewRouterIPNet(r, Direct, Daze)
}
