package main

import (
	"context"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/mohanson/daze/protocol/ashe"
)

const (
	srvListen = "127.0.0.1:2081"
	tcpListen = "127.0.0.1:2083"
	udpListen = "127.0.0.1:2084"
	cipher    = "daze"
)

func mainTCPServer() {
	log.Println("listen and serve on", tcpListen)
	l, err := net.Listen("tcp", tcpListen)
	if err != nil {
		panic(err)
	}
	defer l.Close()
	for {
		c, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			r, w := io.Pipe()
			go io.Copy(w, io.TeeReader(c, os.Stdout))
			io.Copy(c, r)
		}(c)
	}
}

func mainTCPClient() {
	c, err := net.Dial("tcp", tcpListen)
	if err != nil {
		panic(err)
	}
	defer c.Close()
	go io.Copy(os.Stdout, c)
	for range time.NewTicker(time.Second).C {
		m := time.Now().Format(time.RFC1123)
		c.Write([]byte(m + "\n"))
	}
}

func mainTCPDaze() {
	server := ashe.NewServer(srvListen, cipher)
	go server.Run()
	go mainTCPServer()
	time.Sleep(time.Second)
	client := ashe.NewClient(srvListen, cipher)
	c, err := client.Dial(context.Background(), "tcp", tcpListen)
	if err != nil {
		panic(err)
	}
	defer c.Close()
	go io.Copy(os.Stdout, c)
	for range time.NewTicker(time.Second).C {
		m := time.Now().Format(time.RFC1123)
		c.Write([]byte(m + "\n"))
	}
}

func mainUDPServer() {
	log.Println("listen and serve on", udpListen)
	a, err := net.ResolveUDPAddr("udp", udpListen)
	if err != nil {
		panic(err)
	}
	c, err := net.ListenUDP("udp", a)
	if err != nil {
		panic(err)
	}
	defer c.Close()

	b := make([]byte, 1024)
	for {
		n, a, err := c.ReadFromUDP(b)
		if err != nil {
			break
		}
		if n != 30 {
			panic("unreachable")
		}
		os.Stdout.Write(b[:n])
		c.WriteToUDP(b[:n], a)
	}
}

func mainUDPClient() {
	c, err := net.Dial("udp", udpListen)
	if err != nil {
		panic(err)
	}
	defer c.Close()
	go io.Copy(os.Stdout, c)
	for range time.NewTicker(time.Second).C {
		m := time.Now().Format(time.RFC1123)
		c.Write([]byte(m + "\n"))
	}
}

func mainUDPDaze() {
	server := ashe.NewServer(srvListen, cipher)
	go server.Run()
	go mainUDPServer()
	time.Sleep(time.Second)
	client := ashe.NewClient(srvListen, cipher)
	c, err := client.Dial(context.Background(), "udp", udpListen)
	if err != nil {
		panic(err)
	}
	defer c.Close()
	b := make([]byte, 2048)
	go func() {
		for {
			n, err := c.Read(b)
			if err != nil {
				break
			}
			if n != 30 {
				panic("unreachable")
			}
			os.Stdout.Write(b[:n])
		}
	}()
	for range time.NewTicker(time.Second).C {
		m := time.Now().Format(time.RFC1123)
		c.Write([]byte(m + "\n"))
	}
}

func main() {
	flag.Parse()
	switch flag.Arg(0) {
	case "tcp":
		switch flag.Arg(1) {
		case "server":
			mainTCPServer()
		case "client":
			mainTCPClient()
		case "daze":
			mainTCPDaze()
		}
	case "udp":
		switch flag.Arg(1) {
		case "server":
			mainUDPServer()
		case "client":
			mainUDPClient()
		case "daze":
			mainUDPDaze()
		}
	}
}
