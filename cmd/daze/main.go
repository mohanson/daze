package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
	"github.com/mohanson/daze/protocol/ashe"
	"github.com/mohanson/daze/protocol/baboon"
	"github.com/mohanson/daze/protocol/czar"
	"github.com/mohanson/daze/protocol/dahlia"
)

// Conf is acting as package level configuration.
var Conf = struct {
	PathRule string
	PathCIDR string
	Version  string
}{
	PathRule: "/rule.ls",
	PathCIDR: "/rule.cidr",
	Version:  "1.19.4",
}

const helpMsg = `Usage: daze <command> [<args>]

The most commonly used daze commands are:
  server     Start daze server
  client     Start daze client
  gen        Generate or update rule.cidr
  ver        Print the daze version number and exit

Run 'daze <command> -h' for more information on a command.`

const helpGen = `Usage: daze gen <region>

Supported region:
  CN         China

Executing this command will update rule.cidr by remote data source.
`

func main() {
	if len(os.Args) <= 1 {
		fmt.Println(helpMsg)
		return
	}
	// If daze runs in Android through termux, then we set a default dns for it. See:
	// https://stackoverflow.com/questions/38959067/dns-lookup-issue-when-running-my-go-app-in-termux
	if os.Getenv("ANDROID_ROOT") != "" {
		net.DefaultResolver = daze.ResolverDns("1.1.1.1:53")
	}
	resExec := filepath.Dir(doa.Try(os.Executable()))
	subCommand := os.Args[1]
	os.Args = os.Args[1:len(os.Args)]
	switch subCommand {
	case "server":
		var (
			flListen = flag.String("l", "0.0.0.0:1081", "listen address")
			flCipher = flag.String("k", "daze", "password, should be same with the one specified by client")
			flDnserv = flag.String("dns", "", "specifies the DNS, DoT or DoH server")
			flProtoc = flag.String("p", "ashe", "protocol {ashe, baboon, czar, dahlia}")
			flExtend = flag.String("e", "", "extend data for different protocols")
		)
		flag.Parse()
		log.Println("main: server cipher is", *flCipher)
		log.Println("main: protocol is used", *flProtoc)
		if *flDnserv != "" {
			switch {
			case strings.HasSuffix(*flDnserv, ":53"):
				net.DefaultResolver = daze.ResolverDns(*flDnserv)
			case strings.HasSuffix(*flDnserv, ":853"):
				net.DefaultResolver = daze.ResolverDot(*flDnserv)
			case strings.HasPrefix(*flDnserv, "https://"):
				net.DefaultResolver = daze.ResolverDoh(*flDnserv)
			}
			log.Println("main: domain server is", *flDnserv)
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
			defer server.Close()
			doa.Nil(server.Run())
		case "czar":
			server := czar.NewServer(*flListen, *flCipher)
			defer server.Close()
			doa.Nil(server.Run())
		case "dahlia":
			server := dahlia.NewServer(*flListen, *flExtend, *flCipher)
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
			flDnserv = flag.String("dns", "", "specifies the DNS, DoT or DoH server")
			flProtoc = flag.String("p", "ashe", "protocol {ashe, baboon, czar, dahlia}")
		)
		flag.Parse()
		log.Println("main: remote server is", *flServer)
		log.Println("main: client cipher is", *flCipher)
		log.Println("main: protocol is used", *flProtoc)
		if *flDnserv != "" {
			switch {
			case strings.HasSuffix(*flDnserv, ":53"):
				net.DefaultResolver = daze.ResolverDns(*flDnserv)
			case strings.HasSuffix(*flDnserv, ":853"):
				net.DefaultResolver = daze.ResolverDot(*flDnserv)
			case strings.HasPrefix(*flDnserv, "https://"):
				net.DefaultResolver = daze.ResolverDoh(*flDnserv)
			}
			log.Println("main: domain server is", *flDnserv)
		}
		switch *flProtoc {
		case "ashe":
			client := ashe.NewClient(*flServer, *flCipher)
			locale := daze.NewLocale(*flListen, daze.NewAimbot(client, &daze.AimbotOption{
				Type: *flFilter,
				Rule: *flRulels,
				Cidr: *flCIDRls,
			}))
			defer locale.Close()
			doa.Nil(locale.Run())
		case "baboon":
			client := baboon.NewClient(*flServer, *flCipher)
			locale := daze.NewLocale(*flListen, daze.NewAimbot(client, &daze.AimbotOption{
				Type: *flFilter,
				Rule: *flRulels,
				Cidr: *flCIDRls,
			}))
			defer locale.Close()
			doa.Nil(locale.Run())
		case "czar":
			client := czar.NewClient(*flServer, *flCipher)
			locale := daze.NewLocale(*flListen, daze.NewAimbot(client, &daze.AimbotOption{
				Type: *flFilter,
				Rule: *flRulels,
				Cidr: *flCIDRls,
			}))
			defer locale.Close()
			doa.Nil(locale.Run())
		case "dahlia":
			client := dahlia.NewClient(*flListen, *flServer, *flCipher)
			defer client.Close()
			doa.Nil(client.Run())
		}
		daze.Hang()
	case "gen":
		flag.Usage = func() {
			fmt.Fprint(flag.CommandLine.Output(), helpGen)
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
			fmt.Fprintln(f, "L", e.String())
		}
	case "ver":
		fmt.Println("daze", Conf.Version)
	default:
		fmt.Println(helpMsg)
	}
}
