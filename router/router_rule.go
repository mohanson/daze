package router

import (
	"bufio"
	"io"
	"path/filepath"
	"strings"

	"github.com/mohanson/doa"
)

// A single rule in RULE file.
type Rule struct {
	Pattern string
	Road    Road
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
type RouterRule struct {
	rule []Rule
}

// Choose.
func (r *RouterRule) Choose(host string) Road {
	for _, e := range r.rule {
		if doa.Try2(filepath.Match(e.Pattern, host)).(bool) {
			return e.Road
		}
	}
	return Puzzle
}

// Load a RULE file from reader.
func (r *RouterRule) FromReader(f io.Reader) error {
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
				r.rule = append(r.rule, Rule{Pattern: e, Road: Direct})
			}
		case "R":
			for _, e := range seps[1:] {
				r.rule = append(r.rule, Rule{Pattern: e, Road: Daze})
			}
		case "B":
			for _, e := range seps[1:] {
				r.rule = append(r.rule, Rule{Pattern: e, Road: Fucked})
			}
		}
	}
	return scanner.Err()
}

// NewRouterRule returns a new RoaderRule.
func NewRouterRule() *RouterRule {
	return &RouterRule{
		rule: []Rule{},
	}
}
