package codex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	exitByte             = "\x04"
	bracketedPasteStart  = "\x1b[200~"
	bracketedPasteEnd    = "\x1b[201~"
	closeTimeout         = 5 * time.Second
	statePollInterval    = 50 * time.Millisecond
	submitKeyDelay       = 100 * time.Millisecond
	submitRetryInterval  = 2 * time.Second
	submitMaxAttempts    = 3
	stdinPollInterval    = 100 * time.Millisecond
	maxTitleParserBuffer = 1024
)

var errTUIPromptNotAccepted = errors.New("codex tui did not start processing")

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
	mu              sync.Mutex
	idleTitle       string
	parser          titleParser
	idleTransitions int
	busyTransitions int
	currentState    string
}

type titleSnapshot struct {
	idleTransitions int
	busyTransitions int
	currentState    string
}

type titleParser struct {
	pending string
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
		return nil, fmt.Errorf("start codex tui: %w", err)
	}

	session := &tuiSession{
		cmd:       cmd,
		pty:       tty,
		stdout:    c.stdout,
		monitor:   newTitleMonitor(c.workdir),
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
		return fmt.Errorf("codex tui prompt is empty after normalization")
	}

	if err := writeBracketedPaste(s.pty, normalized); err != nil {
		return fmt.Errorf("write prompt to codex tui: %w", err)
	}
	if err := sleepWithContext(ctx, submitKeyDelay); err != nil {
		return err
	}

	submitted := false
	for range submitMaxAttempts {
		if _, err := io.WriteString(s.pty, "\r"); err != nil {
			return fmt.Errorf("submit prompt to codex tui: %w", err)
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
		return fmt.Errorf("%w after %d enter presses", errTUIPromptNotAccepted, submitMaxAttempts)
	}

	if err := s.waitForIdleTransition(ctx, snapshot.idleTransitions, snapshot.busyTransitions); err != nil {
		return fmt.Errorf("wait for codex tui to become idle: %w", err)
	}

	return nil
}

func (s *tuiSession) Close() error {
	select {
	case <-s.done:
		return s.exitErr()
	default:
	}

	_, _ = io.WriteString(s.pty, exitByte)

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
				return false, fmt.Errorf("codex tui exited: %w", err)
			}
			return false, fmt.Errorf("codex tui exited unexpectedly")
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
				return fmt.Errorf("codex tui exited: %w", err)
			}
			return fmt.Errorf("codex tui exited unexpectedly")
		case <-ticker.C:
		}
	}
}

func (s *tuiSession) completeStartup(ctx context.Context) error {
	ticker := time.NewTicker(statePollInterval)
	defer ticker.Stop()

	for {
		snapshot := s.monitor.snapshot()
		if snapshot.readyForPrompt() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.done:
			if err := s.exitErr(); err != nil {
				return fmt.Errorf("codex tui exited: %w", err)
			}
			return fmt.Errorf("codex tui exited unexpectedly")
		case <-ticker.C:
		}
	}
}

func (s *tuiSession) exitErr() error {
	s.waitMu.Lock()
	defer s.waitMu.Unlock()
	return s.waitErr
}

func newTitleMonitor(workdir string) *titleMonitor {
	idleTitle := filepath.Base(filepath.Clean(workdir))
	if idleTitle == "." || idleTitle == string(filepath.Separator) || idleTitle == "" {
		idleTitle = "codex"
	}

	return &titleMonitor{idleTitle: idleTitle}
}

func (m *titleMonitor) consume(chunk []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, title := range m.parser.consume(chunk) {
		state, ok := m.classifyTitle(title)
		if !ok {
			continue
		}
		m.currentState = state
		if state == "idle" {
			m.idleTransitions++
		} else {
			m.busyTransitions++
		}
	}
}

func (m *titleMonitor) snapshot() titleSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	return titleSnapshot{
		idleTransitions: m.idleTransitions,
		busyTransitions: m.busyTransitions,
		currentState:    m.currentState,
	}
}

func (s titleSnapshot) readyForPrompt() bool {
	return s.currentState == "idle" && s.busyTransitions > 0
}

func (m *titleMonitor) classifyTitle(title string) (string, bool) {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return "", false
	}
	if trimmed == m.idleTitle {
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

func normalizePromptForTUI(prompt string) string {
	replaced := strings.ReplaceAll(prompt, "\r\n", "\n")
	replaced = strings.ReplaceAll(replaced, "\r", "\n")
	return strings.Join(strings.Fields(replaced), " ")
}

func writeBracketedPaste(w io.Writer, payload string) error {
	if _, err := io.WriteString(w, bracketedPasteStart); err != nil {
		return err
	}
	if _, err := io.WriteString(w, payload); err != nil {
		return err
	}
	if _, err := io.WriteString(w, bracketedPasteEnd); err != nil {
		return err
	}
	return nil
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
