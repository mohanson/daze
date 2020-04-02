package main

import (
	"flag"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/mohanson/ddir"
)

func call(name string, arg ...string) {
	log.Println("Run", name, strings.Join(arg, " "))
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalln(err)
	}
}

func wget(furl string, name string) {
	log.Println("Get", furl)
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	defer f.Close()
	r, err := http.Get(furl)
	if err != nil {
		log.Fatalln(err)
	}
	defer r.Body.Close()
	io.Copy(f, r.Body)
}

func cp(src string, dst string) {
	log.Println("Cp ", src, dst)
	a, err := os.Open(src)
	if err != nil {
		log.Fatalln(err)
	}
	defer a.Close()
	b, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatalln(err)
	}
	defer b.Close()
	io.Copy(b, a)
}

func main() {
	ddir.Base(".")
	ddir.Make("bin")
	flag.Parse()
	if flag.NArg() == 0 {
		return
	}
	for _, e := range flag.Args() {
		switch e {
		case "develop":
			ddir.Make("bin", "develop")
			if _, err := os.Stat(ddir.Join("bin", "develop", "delegated-apnic-latest")); os.IsNotExist(err) {
				wget("http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest", ddir.Join("bin", "develop", "delegated-apnic-latest"))
			}
			if _, err := os.Stat(ddir.Join("bin", "develop", "rule.ls")); os.IsNotExist(err) {
				cp(ddir.Join("res", "rule.ls"), ddir.Join("bin", "develop", "rule.ls"))
			}
			call("go", "build", "-o", "bin/develop", "github.com/mohanson/daze/cmd/daze")
		case "release":
			ddir.Make("bin", "release")
			wget("http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest", ddir.Join("bin", "release", "delegated-apnic-latest"))
			cp(ddir.Join("res", "rule.ls"), ddir.Join("bin", "release", "rule.ls"))
			call("go", "build", "-o", "bin/release", "github.com/mohanson/daze/cmd/daze")
		}
	}
}
