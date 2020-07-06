package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mohanson/ddir"
)

func call(name string, arg ...string) {
	log.Println("call", name, strings.Join(arg, " "))
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func bash(name string) {
	call("bash", "-c", name)
}

func wget(furl string, name string) {
	log.Println("wget", furl)
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	r, err := http.Get(furl)
	if err != nil {
		panic(err)
	}
	defer r.Body.Close()
	io.Copy(f, r.Body)
}

func cp(src string, dst string) {
	log.Println("copy", src, dst)
	a, err := os.Open(src)
	if err != nil {
		panic(err)
	}
	defer a.Close()
	b, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		panic(err)
	}
	defer b.Close()
	io.Copy(b, a)
}

func main() {
	ddir.Base(".")
	ddir.Make("bin")
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		args = append(args, "develop")
	}
	for _, e := range flag.Args() {
		switch e {
		case "develop":
			ddir.Make("bin")
			if _, err := os.Stat(ddir.Join("bin", "delegated-apnic-latest")); os.IsNotExist(err) {
				wget("http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest", ddir.Join("bin", "delegated-apnic-latest"))
			}
			if _, err := os.Stat(ddir.Join("bin", "rule.ls")); os.IsNotExist(err) {
				cp(ddir.Join("res", "rule.ls"), ddir.Join("bin", "rule.ls"))
			}
			call("go", "build", "-o", "bin", "github.com/mohanson/daze/cmd/daze")
		case "release":
			os.RemoveAll(ddir.Join("bin", "release"))
			ddir.Make("bin", "release")
			ddir.Make("bin", "release", "gox")
			bash("cd bin/release && wget http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest")
			bash("cd bin/release/gox && gox github.com/mohanson/daze/cmd/daze")
			l, _ := filepath.Glob("bin/release/gox/daze_*")
			for _, e := range l {
				b := filepath.Base(e)
				ext := filepath.Ext(b)
				pre := b[0 : len(b)-len(ext)]
				dir := fmt.Sprintf("bin/release/%s", pre)
				bash(fmt.Sprintf("mkdir %s", dir))
				bash(fmt.Sprintf("cp bin/release/gox/%s bin/release/%s/daze%s", b, pre, ext))
				bash(fmt.Sprintf("cp bin/release/delegated-apnic-latest bin/release/%s/delegated-apnic-latest", pre))
				bash(fmt.Sprintf("cp res/rule.ls bin/release/%s/rule.ls", pre))
				bash(fmt.Sprintf("cd bin/release && haze zip -c %s.zip %s", pre, pre))
				bash(fmt.Sprintf("rm -rf bin/release/%s", pre))
			}
			bash("rm -rf bin/release/gox")
			bash("rm bin/release/delegated-apnic-latest")
		}
	}
}
