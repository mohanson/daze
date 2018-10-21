package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
	"github.com/mohanson/daze/protocol/asheshadow"
)

const help = `usage: daze <command> [<args>]

The most commonly used daze commands are:
  server     Start daze server
  client     Start daze client
  cmd        Execute a command by a running client

Run 'daze <command> -h' for more information on a command.`

func printHelpAndExit() {
	fmt.Println(help)
	os.Exit(0)
}

func main() {
	if len(os.Args) <= 1 {
		printHelpAndExit()
	}
	subCommand := os.Args[1]
	os.Args = os.Args[1:len(os.Args)]
	switch subCommand {
	case "server":
		var (
			flListen = flag.String("l", "0.0.0.0:51958", "listen address")
			flCipher = flag.String("k", "daze", "cipher")
			flMasker = flag.String("m", "http://httpbin.org", "")
			flEngine = flag.String("e", "ashe", "engine {ashe, asheshadow}")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
		)
		flag.Parse()
		log.Println("Server cipher is", *flCipher)
		log.Println("Domain server is", *flDnserv)
		if *flDnserv != "" {
			daze.Resolve(*flDnserv)
		}
		switch *flEngine {
		case "ashe":
			server := ashe.NewServer(*flListen, *flCipher)
			if err := server.Run(); err != nil {
				log.Fatalln(err)
			}
		case "asheshadow":
			server := asheshadow.NewServer(*flListen, *flCipher)
			server.Masker = *flMasker
			if err := server.Run(); err != nil {
				log.Fatalln(err)
			}
		default:
			log.Fatalln(*flEngine, "is not an engine")
		}
	case "client":
		var (
			flListen = flag.String("l", "127.0.0.1:51959", "listen address")
			flServer = flag.String("s", "127.0.0.1:51958", "server address")
			flCipher = flag.String("k", "daze", "cipher")
			flEngine = flag.String("e", "ashe", "engine {ashe, asheshadow}")
			flFilter = flag.String("f", "ipcn", "filter {auto, none, ipcn}")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
		)
		flag.Parse()
		log.Println("Remote server is", *flServer)
		log.Println("Client cipher is", *flCipher)
		log.Println("Domain server is", *flDnserv)
		if *flDnserv != "" {
			daze.Resolve(*flDnserv)
		}

		var (
			client daze.Dialer
			filter *daze.Filter
		)
		switch *flEngine {
		case "ashe":
			client = ashe.NewClient(*flServer, *flCipher)
		case "asheshadow":
			client = asheshadow.NewClient(*flServer, *flCipher)
		default:
			log.Fatalln("daze: unknown engine", *flEngine)
		}
		filter = daze.NewFilter(client)
		switch *flFilter {
		case "auto":
			filter.Mold = daze.MoldNier
		case "none":
			filter.Mold = daze.MoldIP
			go func() {
				filter.NetBox.Mrg(daze.IPv4ReservedIPNet())
				filter.NetBox.Mrg(daze.IPv6ReservedIPNet())
			}()
		case "ipcn":
			filter.Mold = daze.MoldIP
			go func() {
				filter.NetBox.Mrg(daze.IPv4ReservedIPNet())
				filter.NetBox.Mrg(daze.IPv6ReservedIPNet())
				filter.NetBox.Mrg(daze.CNIPNet())
			}()
		default:
			log.Fatalln("daze: unknown filter", *flFilter)
		}
		locale := daze.NewLocale(*flListen, filter)
		if err := locale.Run(); err != nil {
			log.Fatalln(err)
		}
	case "cmd":
		var (
			flClient = flag.String("c", "127.0.0.1:51959", "client address")
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
			log.Fatalln(err)
		}
	default:
		printHelpAndExit()
	}
}
