//go:build linux

package claude

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

type terminalInputMode interface {
	Restore() error
	Interactive() bool
}

type noopTerminalInputMode struct{}

func (noopTerminalInputMode) Restore() error     { return nil }
func (noopTerminalInputMode) Interactive() bool  { return false }

type linuxTerminalInputMode struct {
	fd    int
	state *unix.Termios
}

func enterTerminalInputMode(file *os.File) (terminalInputMode, error) {
	if file == nil {
		return noopTerminalInputMode{}, nil
	}

	fd := int(file.Fd())
	state, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		if errors.Is(err, unix.ENOTTY) {
			return noopTerminalInputMode{}, nil
		}
		return nil, err
	}

	updated := *state
	// Drop local echo, canonical line buffering, and extended processing so
	// keystrokes stream straight through to Claude's PTY byte-by-byte. ISIG is
	// left on so Ctrl+C still delivers SIGINT to vibedrive and the normal
	// signal-based shutdown path (which restores termios) runs.
	updated.Lflag &^= unix.ECHO | unix.ICANON | unix.IEXTEN
	updated.Cc[unix.VMIN] = 1
	updated.Cc[unix.VTIME] = 0

	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &updated); err != nil {
		return nil, err
	}

	return &linuxTerminalInputMode{
		fd:    fd,
		state: state,
	}, nil
}

func (m *linuxTerminalInputMode) Restore() error {
	if m == nil || m.state == nil {
		return nil
	}

	return unix.IoctlSetTermios(m.fd, unix.TCSETSF, m.state)
}

func (m *linuxTerminalInputMode) Interactive() bool {
	return m != nil && m.state != nil
}
