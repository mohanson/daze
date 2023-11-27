package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/pprof"
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
	Version:  "v1.19.9",
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
			flDnserv = flag.String("dns", "", "specifies the DNS, DoT or DoH server")
			flExtend = flag.String("e", "", "extend data for different protocols")
			flGpprof = flag.String("g", "", "specify an address to enable net/http/pprof")
			flCipher = flag.String("k", "daze", "password, should be same with the one specified by client")
			flListen = flag.String("l", "0.0.0.0:1081", "listen address")
			flProtoc = flag.String("p", "ashe", "protocol {ashe, baboon, czar, dahlia}")
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
		if *flGpprof != "" {
			_ = pprof.Handler
			log.Println("main: listen net/http/pprof on", *flGpprof)
			go func() { doa.Nil(http.ListenAndServe(*flGpprof, nil)) }()
		}
		daze.Hang()
	case "client":
		var (
			flCIDRls = flag.String("c", filepath.Join(resExec, Conf.PathCIDR), "cidr path")
			flDnserv = flag.String("dns", "", "specifies the DNS, DoT or DoH server")
			flFilter = flag.String("f", "rule", "filter {rule, remote, locale}")
			flGpprof = flag.String("g", "", "specify an address to enable net/http/pprof")
			flCipher = flag.String("k", "daze", "password, should be same with the one specified by server")
			flListen = flag.String("l", "127.0.0.1:1080", "listen address")
			flProtoc = flag.String("p", "ashe", "protocol {ashe, baboon, czar, dahlia}")
			flRulels = flag.String("r", filepath.Join(resExec, Conf.PathRule), "rule path")
			flServer = flag.String("s", "127.0.0.1:1081", "server address")
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
		if *flGpprof != "" {
			_ = pprof.Handler
			log.Println("main: listen net/http/pprof on", *flGpprof)
			go func() { doa.Nil(http.ListenAndServe(*flGpprof, nil)) }()
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
	case "", "-h", "--help":
		fmt.Println(helpMsg)
	}
}
