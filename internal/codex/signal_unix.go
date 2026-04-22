//go:build !windows

package codex

import (
	"os"
	"os/signal"
)

func signalNotify(ch chan<- os.Signal, sig ...os.Signal) {
	signal.Notify(ch, sig...)
}

func signalStop(ch chan<- os.Signal) {
	signal.Stop(ch)
}
