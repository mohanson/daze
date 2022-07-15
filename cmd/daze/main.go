package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/godump/doa"
	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
	"github.com/mohanson/daze/protocol/baboon"
	"github.com/mohanson/daze/protocol/crow"
)

var Conf = struct {
	PathRule string
	PathCIDR string
	Version  string
}{
	PathRule: "/rule.ls",
	PathCIDR: "/rule.cidr",
	Version:  "1.15.8",
}

const HelpMsg = `Usage: daze <command> [<args>]

The most commonly used daze commands are:
  server     Start daze server
  client     Start daze client
  gen        Generate or update rule.cidr
  ver        Print the daze version number and exit

Run 'daze <command> -h' for more information on a command.`

const HelpGen = `Usage: daze gen <region>

Supported region:
  CN         China

If no region is specified, an empty cidr list is generated.
`

func main() {
	if len(os.Args) <= 1 {
		fmt.Println(HelpMsg)
		os.Exit(0)
	}
	// If daze runs in Android through termux, then we set a default dns for it. See:
	// https://stackoverflow.com/questions/38959067/dns-lookup-issue-when-running-my-go-app-in-termux
	if os.Getenv("ANDROID_ROOT") != "" {
		net.DefaultResolver = daze.Resolver("8.8.8.8:53")
	}
	resExec := filepath.Dir(doa.Try(os.Executable()))
	subCommand := os.Args[1]
	os.Args = os.Args[1:len(os.Args)]
	switch subCommand {
	case "server":
		var (
			flListen = flag.String("l", "0.0.0.0:1081", "listen address")
			flCipher = flag.String("k", "daze", "password, should be same with the one specified by client")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
			flProtoc = flag.String("p", "ashe", "protocol {ashe, baboon, crow}")
			flExtend = flag.String("e", "", "extend data for different protocols")
		)
		flag.Parse()
		log.Println("server cipher is", *flCipher)
		log.Println("protocol is used", *flProtoc)
		if *flDnserv != "" {
			daze.Conf.Dialer.Resolver = daze.Resolver(*flDnserv)
			log.Println("domain server is", *flDnserv)
		}
		switch *flProtoc {
		case "ashe":
			server := ashe.NewServer(*flListen, *flCipher)
			defer server.Close()
			doa.Nil(server.Run())
		case "baboon":
			server := baboon.NewServer(*flListen, *flCipher)
			if *flExtend != "" {
				server.Masker = *flExtend
			}
			doa.Nil(server.Run())
		case "crow":
			server := crow.NewServer(*flListen, *flCipher)
			defer server.Close()
			doa.Nil(server.Run())
		}
		daze.Hang()
	case "client":
		var (
			flListen = flag.String("l", "127.0.0.1:1080", "listen address")
			flServer = flag.String("s", "127.0.0.1:1081", "server address")
			flCipher = flag.String("k", "daze", "password, should be same with the one specified by server")
			flFilter = flag.String("f", "rule", "filter {rule, remote, locale}")
			flRulels = flag.String("r", filepath.Join(resExec, Conf.PathRule), "rule path")
			flCIDRls = flag.String("c", filepath.Join(resExec, Conf.PathCIDR), "cidr path")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
			flProtoc = flag.String("p", "ashe", "protocol {ashe, baboon, crow}")
		)
		flag.Parse()
		log.Println("remote server is", *flServer)
		log.Println("client cipher is", *flCipher)
		log.Println("protocol is used", *flProtoc)
		if *flDnserv != "" {
			daze.Conf.Dialer.Resolver = daze.Resolver(*flDnserv)
			log.Println("domain server is", *flDnserv)
		}
		client := func() daze.Dialer {
			switch *flProtoc {
			case "ashe":
				return ashe.NewClient(*flServer, *flCipher)
			case "baboon":
				return baboon.NewClient(*flServer, *flCipher)
			case "crow":
				return crow.NewClient(*flServer, *flCipher)
			}
			panic("unreachable")
		}()
		router := func() daze.Router {
			if *flFilter == "locale" {
				routerRight := daze.NewRouterRight(daze.RoadLocale)
				return routerRight
			}
			if *flFilter == "remote" {
				log.Println("load rule reserved IPv4/6 CIDRs")
				routerLocal := daze.NewRouterLocal()
				log.Println("find", len(routerLocal.L))
				routerRight := daze.NewRouterRight(daze.RoadRemote)
				routerClump := daze.NewRouterClump(routerLocal, routerRight)
				routerCache := daze.NewRouterCache(routerClump)
				return routerCache
			}
			if *flFilter == "rule" {
				log.Println("load rule", *flRulels)
				routerRules := daze.NewRouterRules()
				f1 := doa.Try(daze.OpenFile(*flRulels))
				defer f1.Close()
				doa.Nil(routerRules.FromReader(f1))
				log.Println("find", len(routerRules.L)+len(routerRules.R)+len(routerRules.B))

				log.Println("load rule reserved IPv4/6 CIDRs")
				routerLocal := daze.NewRouterLocal()
				log.Println("find", len(routerLocal.L))

				log.Println("load rule", *flCIDRls)
				f2 := doa.Try(daze.OpenFile(*flCIDRls))
				defer f2.Close()
				routerApnic := daze.NewRouterIPNet([]*net.IPNet{}, daze.RoadLocale)
				routerApnic.FromReader(f2)
				log.Println("find", len(routerApnic.L))

				routerRight := daze.NewRouterRight(daze.RoadRemote)
				routerClump := daze.NewRouterClump(routerRules, routerLocal, routerApnic, routerRight)
				routerCache := daze.NewRouterCache(routerClump)
				return routerCache
			}
			panic("unreachable")
		}()
		aimbot := &daze.Aimbot{
			Remote: client,
			Locale: &daze.Direct{},
			Router: router,
		}
		locale := daze.NewLocale(*flListen, aimbot)
		defer locale.Close()
		doa.Nil(locale.Run())
		daze.Hang()
	case "gen":
		flag.Usage = func() {
			fmt.Fprint(flag.CommandLine.Output(), HelpGen)
			flag.PrintDefaults()
		}
		flag.Parse()
		cidr := func() []*net.IPNet {
			switch strings.ToUpper(flag.Arg(0)) {
			case "CN":
				return daze.LoadApnic()["CN"]
			}
			return []*net.IPNet{}
		}()
		if len(cidr) == 0 {
			flag.Usage()
			return
		}
		f := doa.Try(os.OpenFile(filepath.Join(resExec, Conf.PathCIDR), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644))
		defer f.Close()
		for _, e := range cidr {
			f.WriteString(e.String())
			f.WriteString("\n")
		}
	case "ver":
		fmt.Println("daze", Conf.Version)
	default:
		fmt.Println(HelpMsg)
		os.Exit(0)
	}
}
