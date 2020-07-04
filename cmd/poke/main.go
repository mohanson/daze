package main

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
	"time"
)

var (
	flListen = flag.String("l", ":8080", "listen address")
	flServer = flag.String("s", "127.0.0.1:8080", "server address")
)

func mainTCPServer() {
	log.Println("listen and server on", *flListen)
	l, err := net.Listen("tcp", *flListen)
	if err != nil {
		log.Panicln(err)
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
	c, err := net.Dial("tcp", *flServer)
	if err != nil {
		log.Panicln(err)
	}
	defer c.Close()
	go io.Copy(os.Stdout, c)
	for range time.NewTicker(time.Second).C {
		m := time.Now().Format(time.RFC1123)
		c.Write([]byte(m))
		c.Write([]byte("\n"))
	}
}

func mainUDPServer() {
	log.Println("listen and server on", *flListen)
	a, err := net.ResolveUDPAddr("udp", *flListen)
	if err != nil {
		log.Panicln(err)
	}
	c, err := net.ListenUDP("udp", a)
	if err != nil {
		log.Panicln(err)
	}
	defer c.Close()

	b := make([]byte, 1024)
	for {
		n, a, err := c.ReadFromUDP(b)
		if err != nil {
			break
		}
		os.Stdout.Write(b[:n])
		c.WriteToUDP(b[:n], a)
	}
}

func mainUDPClient() {
	c, err := net.Dial("udp", *flServer)
	if err != nil {
		log.Panicln(err)
	}
	defer c.Close()
	go io.Copy(os.Stdout, c)
	for range time.NewTicker(time.Second).C {
		m := time.Now().Format(time.RFC1123)
		c.Write([]byte(m))
		c.Write([]byte("\n"))
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
		}
	case "udp":
		switch flag.Arg(1) {
		case "server":
			mainUDPServer()
		case "client":
			mainUDPClient()
		}
	}
}
