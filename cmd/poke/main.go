package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"time"
)

const (
	tcpListenPort = 2083
	udpListenPort = 2084
)

func mainTCPServer() {
	tcpListen := fmt.Sprintf(":%d", tcpListenPort)
	log.Println("listen and server on", tcpListen)
	l, err := net.Listen("tcp", tcpListen)
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
	tcpServer := fmt.Sprintf("127.0.0.1:%d", tcpListenPort)
	c, err := net.Dial("tcp", tcpServer)
	if err != nil {
		log.Panicln(err)
	}
	defer c.Close()
	go io.Copy(os.Stdout, c)
	for range time.NewTicker(time.Second).C {
		m := time.Now().Format(time.RFC1123)
		c.Write([]byte(m + "\n"))
	}
}

func mainUDPServer() {
	udpListen := fmt.Sprintf(":%d", udpListenPort)
	log.Println("listen and server on", udpListen)
	a, err := net.ResolveUDPAddr("udp", udpListen)
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
	udpServer := fmt.Sprintf(":%d", udpListenPort)
	c, err := net.Dial("udp", udpServer)
	if err != nil {
		log.Panicln(err)
	}
	defer c.Close()
	go io.Copy(os.Stdout, c)
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
