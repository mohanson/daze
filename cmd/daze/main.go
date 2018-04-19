package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/mohanson/daze"
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
		)
		flag.Parse()
		log.Println("Server cipher is", *flCipher)
		server := daze.NewServer(*flListen, *flCipher)
		if err := server.Run(); err != nil {
			log.Fatalln(err)
		}
	case "client":
		var (
			flListen = flag.String("l", "127.0.0.1:51959", "listen address")
			flServer = flag.String("s", "127.0.0.1:51958", "server address")
			flCipher = flag.String("k", "daze", "cipher")
		)
		flag.Parse()
		log.Println("Remote server is", *flServer)
		log.Println("Client cipher is", *flCipher)
		client := daze.NewClient(*flServer, *flCipher)
		router, err := daze.NewFilter(client)
		if err != nil {
			log.Fatalln(err)
		}
		locale := daze.NewLocale(*flListen, router)
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
		env = append(env, "all_proxy="+*flClient)
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
