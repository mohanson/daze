package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
	"github.com/mohanson/daze/router"
	"github.com/mohanson/easyfs"
)

const help = `usage: daze <command> [<args>]

The most commonly used daze commands are:
  server     Start daze server
  client     Start daze client
  cmd        Execute a command by a running client

Run 'daze <command> -h' for more information on a command.`

func main() {
	if len(os.Args) <= 1 {
		fmt.Println(help)
		os.Exit(0)
	}
	easyfs.BaseExec()
	subCommand := os.Args[1]
	os.Args = os.Args[1:len(os.Args)]
	switch subCommand {
	case "server":
		var (
			flListen = flag.String("l", "0.0.0.0:1081", "listen address")
			flCipher = flag.String("k", "daze", "cipher, for encryption")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
		)
		flag.Parse()
		log.Println("server cipher is", *flCipher)
		if *flDnserv != "" {
			daze.Resolve(*flDnserv)
			log.Println("domain server is", *flDnserv)
		}
		server := ashe.NewServer(*flListen, *flCipher)
		if err := server.Run(); err != nil {
			panic(err)
		}
	case "client":
		var (
			flListen = flag.String("l", "127.0.0.1:1080", "listen address")
			flServer = flag.String("s", "127.0.0.1:1081", "server address")
			flCipher = flag.String("k", "daze", "cipher, for encryption")
			flFilter = flag.String("f", "ipcn", "filter {ipcn, none}")
			flRulels = flag.String("r", easyfs.Path(daze.Conf.PathRule), "rule path")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
		)
		flag.Parse()
		log.Println("remote server is", *flServer)
		log.Println("client cipher is", *flCipher)
		if *flDnserv != "" {
			daze.Resolve(*flDnserv)
			log.Println("domain server is", *flDnserv)
		}
		client := ashe.NewClient(*flServer, *flCipher)
		router := func() router.Router {
			routerCompose := router.NewRouterCompose()

			log.Println("load rule", *flRulels)
			routerRule := router.NewRouterRule()
			f, err := daze.OpenFile(*flRulels)
			if err != nil {
				panic(err)
			}
			defer f.Close()
			if err := routerRule.FromReader(f); err != nil {
				panic(err)
			}
			routerCompose.Join(routerRule)

			log.Println("load rule reserved IPv4/6 CIDRs")
			routerCompose.Join(router.NewRouterReservedIPv4())
			routerCompose.Join(router.NewRouterReservedIPv6())

			if *flFilter == "ipcn" {
				log.Println("load rule CN(China PR) CIDRs")
				f, err := daze.OpenFile(easyfs.Path(daze.Conf.PathDelegatedApnic))
				if err != nil {
					panic(err)
				}
				defer f.Close()
				routerApnic := router.NewRouterApnic(f, "CN")
				log.Println("find", len(routerApnic.Blocks), "IP nets")
				routerCompose.Join(routerApnic)
			}

			routerCompose.Join(router.NewRouterAlways(router.Daze))

			return router.NewRouterLRU(routerCompose)
		}()
		squire := daze.NewSquire(client, router)
		locale := daze.NewLocale(*flListen, squire)
		if err := locale.Run(); err != nil {
			panic(err)
		}
	case "cmd":
		var (
			flClient = flag.String("c", "127.0.0.1:1080", "client address")
		)
		if len(os.Args) <= 1 {
			return
		}
		cmd := exec.Command(os.Args[1], os.Args[2:]...)
		env := os.Environ()
		env = append(env, "all_proxy=socks4a://"+*flClient)
		cmd.Env = env
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			panic(err)
		}
	default:
		fmt.Println(help)
		os.Exit(0)
	}
}
