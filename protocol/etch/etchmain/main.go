package main

import "github.com/mohanson/daze/protocol/etch"

func main() {
	server := etch.NewServer("127.0.0.1:8080")
	server.Run()
	select {}
}
