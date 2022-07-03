package daze

import (
	"bytes"
	"os/exec"
	"testing"

	"github.com/godump/doa"
)

const (
	DazeServerListenOn = "127.0.0.1:28080"
	CurlDest           = "http://www.baidu.com"
)

func TestLocaleHTTP(t *testing.T) {
	locale := NewLocale(DazeServerListenOn, &Direct{})
	defer locale.Close()
	locale.Run()

	cmd := exec.Command("curl", "-x", "http://"+DazeServerListenOn, CurlDest)
	out := doa.Try(cmd.Output())
	if !bytes.Contains(out, []byte("baidu")) {
		t.FailNow()
	}
}

func TestLocaleSocks4(t *testing.T) {
	locale := NewLocale(DazeServerListenOn, &Direct{})
	defer locale.Close()
	locale.Run()

	cmd := exec.Command("curl", "-x", "socks4://"+DazeServerListenOn, CurlDest)
	out := doa.Try(cmd.Output())
	if !bytes.Contains(out, []byte("baidu")) {
		t.FailNow()
	}
}

func TestLocaleSocks4a(t *testing.T) {
	locale := NewLocale(DazeServerListenOn, &Direct{})
	defer locale.Close()
	locale.Run()

	cmd := exec.Command("curl", "-x", "socks4a://"+DazeServerListenOn, CurlDest)
	out := doa.Try(cmd.Output())
	if !bytes.Contains(out, []byte("baidu")) {
		t.FailNow()
	}
}

func TestLocaleSocks5(t *testing.T) {
	locale := NewLocale(DazeServerListenOn, &Direct{})
	defer locale.Close()
	locale.Run()

	cmd := exec.Command("curl", "-x", "socks5://"+DazeServerListenOn, CurlDest)
	out := doa.Try(cmd.Output())
	if !bytes.Contains(out, []byte("baidu")) {
		t.FailNow()
	}
}
