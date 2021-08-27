package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"

	"github.com/godump/doa"
	"github.com/godump/res"
	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
)

var Conf = struct {
	PathDelegatedApnic string
	PathRule           string
	Version            string
}{
	PathDelegatedApnic: "/delegated-apnic-latest",
	PathRule:           "/rule.ls",
	Version:            "1.15.3",
}

const Help = `usage: daze <command> [<args>]

The most commonly used daze commands are:
  server     Start daze server
  client     Start daze client
  ver        Print the daze version number and exit

Run 'daze <command> -h' for more information on a command.`

func main() {
	if len(os.Args) <= 1 {
		fmt.Println(Help)
		os.Exit(0)
	}
	// If daze runs in Android through termux, then we set a default dns for it. See:
	// https://stackoverflow.com/questions/38959067/dns-lookup-issue-when-running-my-go-app-in-termux
	if os.Getenv("ANDROID_ROOT") != "" {
		net.DefaultResolver = daze.Resolver("8.8.8.8:53")
	}
	resExec := res.BaseExec()
	subCommand := os.Args[1]
	os.Args = os.Args[1:len(os.Args)]
	switch subCommand {
	case "server":
		var (
			flListen = flag.String("l", "0.0.0.0:1081", "listen address")
			flCipher = flag.String("k", "daze", "cipher, for encryption, same as client")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
		)
		flag.Parse()
		log.Println("server cipher is", *flCipher)
		if *flDnserv != "" {
			daze.Conf.Dialer.Resolver = daze.Resolver(*flDnserv)
			log.Println("domain server is", *flDnserv)
		}
		server := ashe.NewServer(*flListen, *flCipher)
		doa.Nil(server.Run())
	case "client":
		var (
			flListen = flag.String("l", "127.0.0.1:1080", "listen address")
			flServer = flag.String("s", "127.0.0.1:1081", "server address")
			flCipher = flag.String("k", "daze", "cipher, for encryption, same as server")
			flFilter = flag.String("f", "ipcn", "filter {ipcn, none, full}")
			flRulels = flag.String("r", resExec.Join(Conf.PathRule), "rule path")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
		)
		flag.Parse()
		log.Println("remote server is", *flServer)
		log.Println("client cipher is", *flCipher)
		if *flDnserv != "" {
			daze.Conf.Dialer.Resolver = daze.Resolver(*flDnserv)
			log.Println("domain server is", *flDnserv)
		}
		client := ashe.NewClient(*flServer, *flCipher)
		router := func() daze.Router {
			if *flFilter == "full" {
				routerRight := daze.NewRouterRight(daze.RoadLocale)
				return routerRight
			}
			if *flFilter == "none" {
				log.Println("load rule reserved IPv4/6 CIDRs")
				routerLocal := daze.NewRouterLocal()
				routerRight := daze.NewRouterRight(daze.RoadRemote)
				routerClump := daze.NewRouterClump(routerLocal, routerRight)
				routerCache := daze.NewRouterCache(routerClump)
				return routerCache
			}
			if *flFilter == "ipcn" {
				log.Println("load rule", *flRulels)
				routerRules := daze.NewRouterRules()
				f1 := doa.Try(daze.OpenFile(*flRulels)).(io.ReadCloser)
				defer f1.Close()
				doa.Nil(routerRules.FromReader(f1))
				log.Println("find", len(routerRules.L)+len(routerRules.R)+len(routerRules.B))

				log.Println("load rule reserved IPv4/6 CIDRs")
				routerLocal := daze.NewRouterLocal()
				log.Println("find", len(routerLocal.L))

				log.Println("load rule CN(China PR) CIDRs")
				f2 := doa.Try(daze.OpenFile(resExec.Join(Conf.PathDelegatedApnic))).(io.ReadCloser)
				defer f2.Close()
				routerApnic := daze.NewRouterApnic(f2, "CN")
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
		doa.Nil(locale.Run())
	case "ver":
		fmt.Println("daze", Conf.Version)
	default:
		fmt.Println(Help)
		os.Exit(0)
	}
}
