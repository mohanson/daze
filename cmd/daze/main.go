package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/protocol/ashe"
	"github.com/mohanson/daze/protocol/asheshadow"
	"github.com/mohanson/ddir"
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
	p, err := os.Executable()
	if err != nil {
		log.Panicln(err)
	}
	ddir.Base(filepath.Dir(p))
	subCommand := os.Args[1]
	os.Args = os.Args[1:len(os.Args)]
	switch subCommand {
	case "server":
		var (
			flListen = flag.String("l", "0.0.0.0:1081", "listen address")
			flCipher = flag.String("k", "daze", "cipher, for encryption")
			flMasker = flag.String("m", "http://httpbin.org", "masker, for confusion")
			flEngine = flag.String("e", "ashe", "engine {ashe, asheshadow}")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
		)
		flag.Parse()
		log.Println("server cipher is", *flCipher)
		if *flDnserv != "" {
			daze.Resolve(*flDnserv)
			log.Println("domain server is", *flDnserv)
		}
		switch *flEngine {
		case "ashe":
			server := ashe.NewServer(*flListen, *flCipher)
			if err := server.Run(); err != nil {
				log.Panicln(err)
			}
		case "asheshadow":
			server := asheshadow.NewServer(*flListen, *flCipher)
			server.Masker = *flMasker
			if err := server.Run(); err != nil {
				log.Panicln(err)
			}
		default:
			log.Panicln(*flEngine, "is not an engine")
		}
	case "client":
		var (
			flListen = flag.String("l", "127.0.0.1:1080", "listen address")
			flServer = flag.String("s", "127.0.0.1:1081", "server address")
			flCipher = flag.String("k", "daze", "cipher, for encryption")
			flEngine = flag.String("e", "ashe", "engine {ashe, asheshadow}")
			flRulels = flag.String("r", ddir.Join("rule.ls"), "rule path")
			flDnserv = flag.String("dns", "", "such as 8.8.8.8:53")
			flFilter = flag.String("f", "ipcn", "filter {ipcn, none}")
		)
		flag.Parse()
		if _, err := os.Stat(ddir.Join("rule.ls")); err != nil {
			f, er := os.Create(ddir.Join("rule.ls"))
			if er != nil {
				log.Panicln(er)
			}
			f.Close()
		}
		log.Println("remote server is", *flServer)
		log.Println("client cipher is", *flCipher)
		if *flDnserv != "" {
			daze.Resolve(*flDnserv)
			log.Println("domain server is", *flDnserv)
		}
		var client daze.Dialer
		switch *flEngine {
		case "ashe":
			client = ashe.NewClient(*flServer, *flCipher)
		case "asheshadow":
			client = asheshadow.NewClient(*flServer, *flCipher)
		default:
			log.Panicln("daze: unknown engine", *flEngine)
		}
		squire := daze.NewSquire(client)
		log.Println("load rule", *flRulels)
		if err := squire.Rulels.Load(*flRulels); err != nil {
			log.Panicln(err)
		}
		log.Println("load rule reserved IPv4/6 CIDRs")
		squire.IPNets = append(squire.IPNets, daze.IPv4ReservedIPNet()...)
		squire.IPNets = append(squire.IPNets, daze.IPv6ReservedIPNet()...)
		if *flFilter == "ipcn" {
			log.Println("load rule CN(China PR) CIDRs")
			ipnets := daze.CNIPNet()
			log.Println("find", len(ipnets), "IP nets")
			squire.IPNets = append(squire.IPNets, ipnets...)
		}
		locale := daze.NewLocale(*flListen, squire)
		if err := locale.Run(); err != nil {
			log.Panicln(err)
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
			log.Panicln(err)
		}
	default:
		fmt.Println(help)
		os.Exit(0)
	}
}
