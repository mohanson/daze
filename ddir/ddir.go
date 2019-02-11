package ddir

import (
	"log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
)

var (
	cBase = "./"
	cDone = false
)

func Base(base string) {
	if cDone {
		log.Fatalln("ddir: cannot call Base() multiple times")
	}
	cBase = base
	cDone = true
}

func Auto(name string) {
	if runtime.GOOS == "windows" {
		Base(filepath.Join(os.Getenv("localappdata"), name))
		return
	}
	if runtime.GOOS == "linux" && runtime.GOARCH == "arm" {
		return
	}
	u, err := user.Current()
	if err != nil {
		log.Fatalln(err)
	}
	Base(filepath.Join(u.HomeDir, "."+name))
}

func Join(elem ...string) string {
	return filepath.Join(append([]string{cBase}, elem...)...)
}

func Make(elem ...string) {
	err := os.Mkdir(Join(elem...), 0755)
	if err != nil && !os.IsExist(err) {
		log.Fatalln(err)
	}
}
