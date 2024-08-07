// A graceful exit (or graceful handling) is a simple programming idiom[citation needed] wherein a program detects a
// serious error condition and "exits gracefully" in a controlled manner as a result. Often the program prints a
// descriptive error message to a terminal or log as part of the graceful exit.
package gracefulexit

import (
	"os"
	"os/signal"
	"syscall"
)

// Chan create a channel for os.Signal.
func Chan() chan os.Signal {
	buffer := make(chan os.Signal, 1)
	signal.Notify(buffer, syscall.SIGINT, syscall.SIGTERM)
	return buffer
}

// Wait for a signal.
func Wait() {
	<-Chan()
}
