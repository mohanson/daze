package daze

import (
	"bufio"
	"context"
	"crypto/cipher"
	"crypto/rc4"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/user"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func Link(a, b io.ReadWriteCloser) {
	go func() {
		io.Copy(b, a)
		a.Close()
		b.Close()
	}()
	io.Copy(a, b)
	b.Close()
	a.Close()
}

type ReadWriteCloser struct {
	io.Reader
	io.Writer
	io.Closer
}

func GravityReader(r io.Reader, k []byte) io.Reader {
	cr, _ := rc4.NewCipher(k)
	return cipher.StreamReader{S: cr, R: r}
}

func GravityWriter(w io.Writer, k []byte) io.Writer {
	cw, _ := rc4.NewCipher(k)
	return cipher.StreamWriter{S: cw, W: w}
}

func Gravity(conn io.ReadWriteCloser, k []byte) io.ReadWriteCloser {
	cr, _ := rc4.NewCipher(k)
	cw, _ := rc4.NewCipher(k)
	return &ReadWriteCloser{
		Reader: cipher.StreamReader{S: cr, R: conn},
		Writer: cipher.StreamWriter{S: cw, W: conn},
		Closer: conn,
	}
}

type Dialer interface {
	Dial(network, address string) (io.ReadWriteCloser, error)
}

type NetBox struct {
	L []*net.IPNet
}

func (n *NetBox) Add(ipNet *net.IPNet) {
	n.L = append(n.L, ipNet)
}

func (n *NetBox) Has(ip net.IP) bool {
	for _, entry := range n.L {
		if entry.Contains(ip) {
			return true
		}
	}
	return false
}

func IPv4ReservedIPNet() *NetBox {
	netBox := &NetBox{}
	for _, entry := range [][2]string{
		[2]string{"00000000", "FF000000"},
		[2]string{"0A000000", "FF000000"},
		[2]string{"7F000000", "FF000000"},
		[2]string{"A9FE0000", "FFFF0000"},
		[2]string{"AC100000", "FFF00000"},
		[2]string{"C0000000", "FFFFFFF8"},
		[2]string{"C00000AA", "FFFFFFFE"},
		[2]string{"C0000200", "FFFFFF00"},
		[2]string{"C0A80000", "FFFF0000"},
		[2]string{"C6120000", "FFFE0000"},
		[2]string{"C6336400", "FFFFFF00"},
		[2]string{"CB007100", "FFFFFF00"},
		[2]string{"F0000000", "F0000000"},
		[2]string{"FFFFFFFF", "FFFFFFFF"},
	} {
		i, _ := hex.DecodeString(entry[0])
		m, _ := hex.DecodeString(entry[1])
		netBox.Add(&net.IPNet{IP: i, Mask: m})
	}
	return netBox
}

func IPv6ReservedIPNet() *NetBox {
	netBox := &NetBox{}
	for _, entry := range [][2]string{
		[2]string{"00000000000000000000000000000000", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"},
		[2]string{"00000000000000000000000000000001", "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"},
		[2]string{"00000000000000000000FFFF00000000", "FFFFFFFFFFFFFFFFFFFFFFFF00000000"},
		[2]string{"01000000000000000000000000000000", "FFFFFFFFFFFFFFFF0000000000000000"},
		[2]string{"0064FF9B000000000000000000000000", "FFFFFFFFFFFFFFFFFFFFFFFF00000000"},
		[2]string{"20010000000000000000000000000000", "FFFFFFFF000000000000000000000000"},
		[2]string{"20010010000000000000000000000000", "FFFFFFF0000000000000000000000000"},
		[2]string{"20010020000000000000000000000000", "FFFFFFF0000000000000000000000000"},
		[2]string{"20010DB8000000000000000000000000", "FFFFFFFF000000000000000000000000"},
		[2]string{"20020000000000000000000000000000", "FFFF0000000000000000000000000000"},
		[2]string{"FC000000000000000000000000000000", "FE000000000000000000000000000000"},
		[2]string{"FE800000000000000000000000000000", "FFC00000000000000000000000000000"},
		[2]string{"FF000000000000000000000000000000", "FF000000000000000000000000000000"},
	} {
		i, _ := hex.DecodeString(entry[0])
		m, _ := hex.DecodeString(entry[1])
		netBox.Add(&net.IPNet{IP: i, Mask: m})
	}
	return netBox
}

func DarkMainlandIPNet() *NetBox {
	if err := os.MkdirAll(DataPath, 0666); err != nil {
		log.Fatalln(err)
	}
	var reader io.Reader
	filePath := path.Join(DataPath, "delegated-apnic-latest")
	fileInfo, err := os.Stat(filePath)
	if err != nil || time.Since(fileInfo.ModTime()) > time.Hour*24*28 {
		r, err := http.Get("http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest")
		if err != nil {
			log.Fatalln(err)
		}
		defer r.Body.Close()

		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
		if err != nil {
			log.Fatalln(err)
		}
		defer file.Close()

		reader = io.TeeReader(r.Body, file)
	} else {
		file, err := os.Open(filePath)
		if err != nil {
			log.Fatalln(err)
		}
		defer file.Close()

		reader = file
	}

	netBox := &NetBox{}
	s := bufio.NewScanner(reader)
	for s.Scan() {
		line := s.Text()
		if !strings.HasPrefix(line, "apnic|CN|ipv4") {
			continue
		}
		seps := strings.Split(line, "|")
		sep4, err := strconv.Atoi(seps[4])
		if err != nil {
			log.Fatalln(err)
		}
		mask := 32 - int(math.Log2(float64(sep4)))
		_, cidr, err := net.ParseCIDR(fmt.Sprintf("%s/%d", seps[3], mask))
		if err != nil {
			log.Fatalln(err)
		}
		netBox.Add(cidr)
	}

	return netBox
}

var DataPath = func() string {
	var data string
	if runtime.GOOS == "windows" {
		data = path.Join(os.Getenv("localappdata"), "daze")
	} else {
		u, _ := user.Current()
		data = path.Join(u.HomeDir, ".daze")
	}
	return data
}()

func ChangeDefaultResolver(addr string) {
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", addr)
		},
	}
}
