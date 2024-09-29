package etch

import (
	"testing"
	"time"
)

const (
	DazeServerListenOn = "127.0.0.1:28081"
)

func TestProtocolEtch(t *testing.T) {
	server := NewServer(DazeServerListenOn)
	go server.Run()
	time.Sleep(time.Second)

	client := NewClient(DazeServerListenOn)
	client.Run()
	time.Sleep(time.Second)
}
