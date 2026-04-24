package claude

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	exitCommand          = "/exit\r"
	closeTimeout         = 5 * time.Second
	statePollInterval    = 50 * time.Millisecond
	submitKeyDelay       = 100 * time.Millisecond
	submitRetryInterval  = 2 * time.Second
	submitMaxAttempts    = 3
	stdinPollInterval    = 100 * time.Millisecond
	maxTitleParserBuffer = 1024
)

type tuiSession struct {
	cmd       *exec.Cmd
	pty       *os.File
	stdout    io.Writer
	monitor   *titleMonitor
	inputMode terminalInputMode
	done      chan struct{}
	doneOnce  sync.Once
	waitErr   error
	waitMu    sync.Mutex
	resizeSig chan os.Signal
}

type titleMonitor struct {
	mu                  sync.Mutex
	parser              titleParser
	text                visibleTextParser
	idleTransitions     int
	busyTransitions     int
	trustPrompts        int
	trustPromptDetected bool
}

type titleSnapshot struct {
	idleTransitions int
	busyTransitions int
	trustPrompts    int
}

type titleParser struct {
	pending string
}

type visibleTextParser struct {
	recent string
}

func (c *Client) startTUI(ctx context.Context) (*tuiSession, error) {
	inputMode, err := enterTerminalInputMode(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("prepare terminal input: %w", err)
	}

	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Dir = c.workdir

	tty, err := pty.Start(cmd)
	if err != nil {
		_ = inputMode.Restore()
		return nil, fmt.Errorf("start claude tui: %w", err)
	}

	session := &tuiSession{
		cmd:       cmd,
		pty:       tty,
		stdout:    c.stdout,
		monitor:   &titleMonitor{},
		inputMode: inputMode,
		done:      make(chan struct{}),
		resizeSig: make(chan os.Signal, 1),
	}

	session.startOutputPump()
	session.startInputPump()
	session.startWaiter()
	session.startResizeForwarder()

	readyCtx, cancel := context.WithTimeout(ctx, c.startupTimeout)
	defer cancel()

	if err := session.completeStartup(readyCtx); err != nil {
		_ = session.Close()
		return nil, err
	}

	return session, nil
}

func (s *tuiSession) SendPrompt(ctx context.Context, prompt string) error {
	snapshot := s.monitor.snapshot()

	normalized := normalizePromptForTUI(prompt)
	if normalized == "" {
		return fmt.Errorf("claude tui prompt is empty after normalization")
	}

	if _, err := io.WriteString(s.pty, normalized); err != nil {
		return fmt.Errorf("write prompt to claude tui: %w", err)
	}
	if err := sleepWithContext(ctx, submitKeyDelay); err != nil {
		return err
	}

	// Press Enter and wait briefly for Claude to start processing. If it
	// doesn't, the Enter likely got eaten by paste-bracketing or composer
	// state — retry a small number of times before giving up so we never
	// hang forever on a silently-dropped submit.
	submitted := false
	for range submitMaxAttempts {
		if _, err := io.WriteString(s.pty, "\r"); err != nil {
			return fmt.Errorf("submit prompt to claude tui: %w", err)
		}
		busy, err := s.waitForBusyTransition(ctx, snapshot.busyTransitions, submitRetryInterval)
		if err != nil {
			return err
		}
		if busy {
			submitted = true
			break
		}
	}
	if !submitted {
		return fmt.Errorf("claude tui did not start processing after %d enter presses", submitMaxAttempts)
	}

	if err := s.waitForIdleTransition(ctx, snapshot.idleTransitions, snapshot.busyTransitions); err != nil {
		return fmt.Errorf("wait for claude tui to become idle: %w", err)
	}

	return nil
}

func (s *tuiSession) Close() error {
	select {
	case <-s.done:
		return s.exitErr()
	default:
	}

	_, _ = io.WriteString(s.pty, exitCommand)

	timer := time.NewTimer(closeTimeout)
	defer timer.Stop()

	select {
	case <-s.done:
		return s.exitErr()
	case <-timer.C:
		if s.cmd.Process != nil {
			_ = s.cmd.Process.Kill()
		}
		<-s.done
		return s.exitErr()
	}
}

func (s *tuiSession) startOutputPump() {
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := s.pty.Read(buffer)
			if n > 0 {
				chunk := append([]byte(nil), buffer[:n]...)
				s.monitor.consume(chunk)
				if s.stdout != nil {
					_, _ = s.stdout.Write(chunk)
				}
			}
			if err != nil {
				return
			}
		}
	}()
}

// startInputPump forwards the user's real stdin keystrokes into the Claude
// PTY. Without this, vibedrive captures stdin but never relays it, leaving
// users with no way to intervene (or recover their terminal) when a step
// hangs. Bytes stream through raw since enterTerminalInputMode disables
// ICANON/ECHO on Linux.
//
// Uses SetReadDeadline in a poll loop so the goroutine can exit promptly when
// the session closes. On platforms/fds that don't support deadlines the pump
// silently steps down and the session still runs — just without the manual
// intervention escape hatch.
func (s *tuiSession) startInputPump() {
	if !s.inputMode.Interactive() {
		return
	}

	go func() {
		defer func() { _ = os.Stdin.SetReadDeadline(time.Time{}) }()

		buffer := make([]byte, 256)
		for {
			select {
			case <-s.done:
				return
			default:
			}

			if err := os.Stdin.SetReadDeadline(time.Now().Add(stdinPollInterval)); err != nil {
				return
			}

			n, err := os.Stdin.Read(buffer)
			if n > 0 {
				if _, werr := s.pty.Write(buffer[:n]); werr != nil {
					return
				}
			}
			if err == nil {
				continue
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				continue
			}
			return
		}
	}()
}

func (s *tuiSession) startWaiter() {
	go func() {
		err := s.cmd.Wait()
		_ = s.pty.Close()
		_ = s.inputMode.Restore()

		s.waitMu.Lock()
		s.waitErr = err
		s.waitMu.Unlock()

		s.doneOnce.Do(func() {
			close(s.done)
		})
	}()
}

func (s *tuiSession) startResizeForwarder() {
	if _, err := pty.GetsizeFull(os.Stdout); err != nil {
		return
	}

	signalNotify(s.resizeSig, syscall.SIGWINCH)
	_ = pty.InheritSize(os.Stdout, s.pty)

	go func() {
		defer signalStop(s.resizeSig)
		for {
			select {
			case <-s.done:
				return
			case <-s.resizeSig:
				_ = pty.InheritSize(os.Stdout, s.pty)
			}
		}
	}()
}

// waitForBusyTransition polls the title monitor until Claude transitions to a
// busy state (indicating it accepted our submit) or the timeout elapses.
// Returns (false, nil) on timeout — the caller decides whether to retry.
func (s *tuiSession) waitForBusyTransition(ctx context.Context, busyStart int, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(statePollInterval)
	defer ticker.Stop()

	for {
		snapshot := s.monitor.snapshot()
		if snapshot.busyTransitions > busyStart {
			return true, nil
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-s.done:
			if err := s.exitErr(); err != nil {
				return false, fmt.Errorf("claude tui exited: %w", err)
			}
			return false, fmt.Errorf("claude tui exited unexpectedly")
		case <-ticker.C:
		}
	}
}

func (s *tuiSession) waitForIdleTransition(ctx context.Context, idleStart, busyStart int) error {
	ticker := time.NewTicker(statePollInterval)
	defer ticker.Stop()

	for {
		snapshot := s.monitor.snapshot()
		if snapshot.busyTransitions > busyStart && snapshot.idleTransitions > idleStart {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			if err := s.exitErr(); err != nil {
				return fmt.Errorf("claude tui exited: %w", err)
			}
			return fmt.Errorf("claude tui exited unexpectedly")
		case <-ticker.C:
		}
	}
}

func (s *tuiSession) completeStartup(ctx context.Context) error {
	ticker := time.NewTicker(statePollInterval)
	defer ticker.Stop()

	handledTrustPrompts := 0

	for {
		snapshot := s.monitor.snapshot()
		if snapshot.idleTransitions > 0 {
			return nil
		}
		if snapshot.trustPrompts > handledTrustPrompts {
			if _, err := io.WriteString(s.pty, "\r"); err != nil {
				return fmt.Errorf("confirm claude trust dialog: %w", err)
			}
			handledTrustPrompts = snapshot.trustPrompts
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			if err := s.exitErr(); err != nil {
				return fmt.Errorf("claude tui exited: %w", err)
			}
			return fmt.Errorf("claude tui exited unexpectedly")
		case <-ticker.C:
		}
	}
}

func (s *tuiSession) exitErr() error {
	s.waitMu.Lock()
	defer s.waitMu.Unlock()
	return s.waitErr
}

func (m *titleMonitor) consume(chunk []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, title := range m.parser.consume(chunk) {
		state, ok := classifyTitle(title)
		if !ok {
			continue
		}
		if state == "idle" {
			m.idleTransitions++
		} else {
			m.busyTransitions++
		}
	}

	recentVisible := m.text.consume(chunk)
	trustDetected := strings.Contains(recentVisible, "yesitrustthisfolder")
	if trustDetected && !m.trustPromptDetected {
		m.trustPrompts++
	}
	m.trustPromptDetected = trustDetected
}

func (m *titleMonitor) snapshot() titleSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	return titleSnapshot{
		idleTransitions: m.idleTransitions,
		busyTransitions: m.busyTransitions,
		trustPrompts:    m.trustPrompts,
	}
}

func classifyTitle(title string) (string, bool) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(trimmed, "✳ ") {
		return "idle", true
	}
	return "busy", true
}

func (p *titleParser) consume(chunk []byte) []string {
	p.pending += string(chunk)

	var titles []string
	for {
		start := strings.Index(p.pending, "\x1b]0;")
		if start == -1 {
			p.trim()
			return titles
		}

		if start > 0 {
			p.pending = p.pending[start:]
		}

		body := p.pending[len("\x1b]0;"):]
		belIndex := strings.Index(body, "\x07")
		stIndex := strings.Index(body, "\x1b\\")

		endIndex := -1
		terminatorLength := 0
		switch {
		case belIndex >= 0 && (stIndex == -1 || belIndex < stIndex):
			endIndex = belIndex
			terminatorLength = 1
		case stIndex >= 0:
			endIndex = stIndex
			terminatorLength = 2
		default:
			p.trim()
			return titles
		}

		titles = append(titles, body[:endIndex])
		p.pending = body[endIndex+terminatorLength:]
	}
}

func (p *titleParser) trim() {
	if len(p.pending) > maxTitleParserBuffer {
		p.pending = p.pending[len(p.pending)-maxTitleParserBuffer:]
	}
}

func (p *visibleTextParser) consume(chunk []byte) string {
	p.recent += compactVisibleText(chunk)
	if len(p.recent) > maxTitleParserBuffer {
		p.recent = p.recent[len(p.recent)-maxTitleParserBuffer:]
	}
	return p.recent
}

func compactVisibleText(chunk []byte) string {
	var out strings.Builder

	for i := 0; i < len(chunk); {
		if chunk[i] == 0x1b {
			i++
			if i >= len(chunk) {
				break
			}

			switch chunk[i] {
			case '[':
				i++
				for i < len(chunk) && ((chunk[i] >= 0x30 && chunk[i] <= 0x3f) || (chunk[i] >= 0x20 && chunk[i] <= 0x2f)) {
					i++
				}
				if i < len(chunk) {
					i++
				}
			case ']':
				i++
				for i < len(chunk) {
					if chunk[i] == 0x07 {
						i++
						break
					}
					if chunk[i] == 0x1b && i+1 < len(chunk) && chunk[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			default:
				i++
			}
			continue
		}

		r := rune(chunk[i])
		i++

		if r >= 'A' && r <= 'Z' {
			out.WriteRune(r + ('a' - 'A'))
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out.WriteRune(r)
		}
	}

	return out.String()
}

func normalizePromptForTUI(prompt string) string {
	replaced := strings.ReplaceAll(prompt, "\r\n", "\n")
	replaced = strings.ReplaceAll(replaced, "\r", "\n")
	return strings.Join(strings.Fields(replaced), " ")
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
