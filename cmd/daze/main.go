package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/godump/ddir"
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
	switch {
	case runtime.GOOS == "windows":
		ddir.Base(filepath.Join(os.Getenv("localappdata"), "Daze"))
	case runtime.GOOS == "linux" && runtime.GOARCH == "arm":
		ddir.Base(".")
	default:
		ddir.Base(filepath.Join(ddir.Home(), ".daze"))
	}
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
		log.Println("Server cipher is", *flCipher)
		if *flDnserv != "" {
			daze.Resolve(*flDnserv)
			log.Println("Domain server is", *flDnserv)
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
		)
		flag.Parse()
		if _, err := os.Stat(ddir.Join("rule.ls")); err != nil {
			f, er := os.Create(ddir.Join("rule.ls"))
			if er != nil {
				log.Panicln(er)
			}
			f.Close()
		}
		log.Println("Remote server is", *flServer)
		log.Println("Client cipher is", *flCipher)
		if *flDnserv != "" {
			daze.Resolve(*flDnserv)
			log.Println("Domain server is", *flDnserv)
		}
		var client daze.Dialer
		switch *flEngine {
		case "ashe":
			client = ashe.NewClient(*flServer, *flCipher)
		case "asheshadow":
			client = asheshadow.NewClient(*flServer, *flCipher)
		}
		if client == nil {
			log.Panicln("daze: unknown engine", *flEngine)
		}
		squire := daze.NewSquire(client)
		log.Println("Load rule", *flRulels)
		if err := squire.Rulels.Load(*flRulels); err != nil {
			log.Panicln(err)
		}
		log.Println("Load rule reserved IPv4/6 CIDRs")
		squire.IPNets = append(squire.IPNets, daze.IPv4ReservedIPNet()...)
		squire.IPNets = append(squire.IPNets, daze.IPv6ReservedIPNet()...)
		log.Println("Load rule CN(China PR) CIDRs")
		go func() {
			time.Sleep(4 * time.Second)
			os.Setenv("HTTP_PROXY", "http://"+*flListen)
			squire.IPNets = append(squire.IPNets, daze.CNIPNet()...)
			os.Setenv("HTTP_PROXY", "")
		}()
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
		printHelpAndExit()
	}
}
