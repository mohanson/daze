package main

import (
	"io"
	"log"
	"os"
	"time"

	"github.com/mohanson/daze/protocol/ashe"
)

const (
	asheSrvListen = "127.0.0.1:2081"
	pokeTCPServer = "127.0.0.1:2083"
	pokeUDPServer = "127.0.0.1:2084"
	cipher        = "daze"
)

func mainTCP() {
	client := ashe.NewClient(asheSrvListen, cipher)
	c, err := client.Dial("tcp", pokeTCPServer)
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

func mainUDP() {
	client := ashe.NewClient(asheSrvListen, cipher)
	c, err := client.Dial("udp", pokeUDPServer)
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
	server := ashe.NewServer(asheSrvListen, cipher)
	go server.Run()
	time.Sleep(time.Second)
	go mainUDP()
	go mainTCP()
	select {}
}
