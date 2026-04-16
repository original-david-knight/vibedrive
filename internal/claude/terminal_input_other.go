//go:build !linux

package claude

import "os"

type terminalInputMode interface {
	Restore() error
	Interactive() bool
}

type noopTerminalInputMode struct{}

func (noopTerminalInputMode) Restore() error    { return nil }
func (noopTerminalInputMode) Interactive() bool { return false }

func enterTerminalInputMode(_ *os.File) (terminalInputMode, error) {
	return noopTerminalInputMode{}, nil
}
