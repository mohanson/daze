package main

import (
	"github.com/mohanson/daze/lib/pretty"
)

func main() {
	pretty.PrintTable([][]string{
		{"City name", "Area", "Population", "Annual Rainfall"},
		{"Adelaide", "1295", "1158259", "600.5"},
		{"Brisbane", "5905", "1857594", "1146.4"},
		{"Darwin", "112", "120900", "1714.7"},
		{"Hobart", "1357", "205556", "619.5"},
		{"Melbourne", "1566", "3806092", "646.9"},
		{"Perth", "5386", "1554769", "869.4"},
		{"Sydney", "2058", "4336374", "1214.8"},
	})
}
