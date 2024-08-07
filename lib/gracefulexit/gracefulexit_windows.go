package gracefulexit

import (
	"os"
	"os/signal"
)

// Chan create a channel for os.Signal.
func Chan() chan os.Signal {
	buffer := make(chan os.Signal, 1)
	signal.Notify(buffer, os.Interrupt)
	return buffer
}

// Wait for a signal.
func Wait() {
	<-Chan()
}
