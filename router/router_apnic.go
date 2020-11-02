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
			sep4, err := strconv.Atoi(seps[4])
			if err != nil {
				panic(err)
			}
			mask := 32 - int(math.Log2(float64(sep4)))
			_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%d", seps[3], mask))
			if err != nil {
				panic(err)
			}
			r = append(r, cidr)
		case strings.HasPrefix(line, ipv6Prefix):
			seps := strings.Split(line, "|")
			sep4 := seps[4]
			_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%s", seps[3], sep4))
			if err != nil {
				panic(err)
			}
			r = append(r, cidr)
		}
	}
	return NewRouterIPNet(r, Direct, Daze)
}
