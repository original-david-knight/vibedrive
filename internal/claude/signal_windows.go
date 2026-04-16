//go:build windows

package claude

import "os"

func signalNotify(ch chan<- os.Signal, sig ...os.Signal) {}

func signalStop(ch chan<- os.Signal) {}
