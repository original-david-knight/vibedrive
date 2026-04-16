package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ghost_claude/internal/claude"
	"ghost_claude/internal/config"
	"ghost_claude/internal/render"
	"ghost_claude/internal/todo"
)

type Runner struct {
	cfg    *config.Config
	stdout io.Writer
	stderr io.Writer
	agent  agentClient

	newSession func(string) (*claude.Session, error)
}

type TemplateData struct {
	ConfigPath string
	Iteration  int
	SessionID  string
	Workspace  string
	TodoFile   string
	NextTodo   todo.Item
	Now        time.Time
}

type agentClient interface {
	RunPrompt(ctx context.Context, session *claude.Session, prompt string) error
	Close(session *claude.Session) error
	IsFullscreenTUI() bool
}

func New(cfg *config.Config, stdout, stderr io.Writer) (*Runner, error) {
	agent, err := claude.New(
		cfg.Claude.Command,
		cfg.Claude.Args,
		cfg.Workspace,
		cfg.Claude.Transport,
		cfg.Claude.StartupTimeout,
		stdout,
		stderr,
	)
	if err != nil {
		return nil, err
	}

	return &Runner{
		cfg:    cfg,
		stdout: stdout,
		stderr: stderr,
		agent:  agent,
		newSession: func(strategy string) (*claude.Session, error) {
			return claude.NewSession(strategy)
		},
	}, nil
}

func (r *Runner) Run(ctx context.Context) (err error) {
	stalled := 0

	for iteration := 1; ; iteration++ {
		if r.cfg.MaxIterations > 0 && iteration > r.cfg.MaxIterations {
			return fmt.Errorf("stopped after reaching max_iterations=%d", r.cfg.MaxIterations)
		}

		nextTodo, err := todo.FindNextIncomplete(r.cfg.TodoFile)
		if err != nil {
			if errors.Is(err, todo.ErrNoIncompleteItems) {
				if r.shouldLogProgress() {
					fmt.Fprintln(r.stdout, "No incomplete TODO items remain.")
				}
				return nil
			}
			return err
		}

		if r.shouldLogProgress() {
			fmt.Fprintf(r.stdout, "\n== Iteration %d ==\n", iteration)
			fmt.Fprintf(r.stdout, "Next TODO: %s (line %d)\n", nextTodo.Text, nextTodo.Line)
		}

		session, err := r.createSession()
		if err != nil {
			return err
		}

		data := TemplateData{
			ConfigPath: r.cfg.Path,
			Iteration:  iteration,
			SessionID:  session.ID,
			Workspace:  r.cfg.Workspace,
			TodoFile:   r.cfg.TodoFile,
			NextTodo:   nextTodo,
			Now:        time.Now(),
		}

		var stepErr error
		for _, step := range r.cfg.Steps {
			if step.Disabled {
				continue
			}

			if err := r.runStep(ctx, session, step, data); err != nil {
				if step.ContinueOnError {
					if r.shouldLogProgress() {
						fmt.Fprintf(r.stderr, "warning: step %q failed but continue_on_error is set: %v\n", step.Name, err)
					}
					continue
				}
				stepErr = fmt.Errorf("step %q failed: %w", step.Name, err)
				break
			}
		}

		closeErr := r.agent.Close(session)
		if stepErr != nil {
			if closeErr != nil {
				return fmt.Errorf("%w; also failed to close claude session: %v", stepErr, closeErr)
			}
			return stepErr
		}
		if closeErr != nil {
			return closeErr
		}

		if r.cfg.DryRun {
			fmt.Fprintln(r.stdout, "\nDry run complete.")
			return nil
		}

		nextAfterIteration, err := todo.FindNextIncomplete(r.cfg.TodoFile)
		if err != nil {
			if errors.Is(err, todo.ErrNoIncompleteItems) {
				if r.shouldLogProgress() {
					fmt.Fprintln(r.stdout, "\nAll TODO items are complete.")
				}
				return nil
			}
			return err
		}

		if nextAfterIteration.Signature() == nextTodo.Signature() {
			stalled++
			if stalled >= r.cfg.MaxStalledIterations {
				return fmt.Errorf(
					"iteration %d made no TODO progress; %q is still the next incomplete item. "+
						"ghost-claude only advances when the first incomplete checkbox changes in %s. "+
						"This usually means no step edited the TODO file. "+
						"Raise max_stalled_iterations if you want automatic retries on the same item",
					iteration,
					nextAfterIteration.Text,
					r.cfg.TodoFile,
				)
			}
			if r.shouldLogProgress() {
				fmt.Fprintf(r.stderr, "warning: no TODO progress after iteration %d; retrying (%d/%d)\n", iteration, stalled, r.cfg.MaxStalledIterations)
			}
		} else {
			stalled = 0
		}
	}
}

func (r *Runner) createSession() (*claude.Session, error) {
	if r.newSession != nil {
		return r.newSession(r.cfg.Claude.SessionStrategy)
	}
	return claude.NewSession(r.cfg.Claude.SessionStrategy)
}

func (r *Runner) runStep(ctx context.Context, session *claude.Session, step config.Step, data TemplateData) error {
	stepCtx := ctx
	var cancel context.CancelFunc
	if step.Timeout != "" {
		timeout, err := time.ParseDuration(step.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %w", step.Timeout, err)
		}
		stepCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	switch strings.ToLower(step.Type) {
	case config.StepTypeClaude:
		prompt, err := render.String(step.Prompt, data)
		if err != nil {
			return fmt.Errorf("render prompt: %w", err)
		}

		if r.shouldLogProgress() {
			fmt.Fprintf(r.stdout, "\n--> claude step: %s\n", step.Name)
		}
		if r.cfg.DryRun {
			fmt.Fprintln(r.stdout, strings.TrimSpace(prompt))
			return nil
		}
		return r.agent.RunPrompt(stepCtx, session, prompt)
	case config.StepTypeExec:
		command, err := render.Strings(step.Command, data)
		if err != nil {
			return fmt.Errorf("render command: %w", err)
		}
		if len(command) == 0 {
			return fmt.Errorf("rendered command is empty")
		}

		workdir := r.cfg.Workspace
		if step.WorkingDir != "" {
			workdir, err = render.String(step.WorkingDir, data)
			if err != nil {
				return fmt.Errorf("render working_dir: %w", err)
			}
			if !filepath.IsAbs(workdir) {
				workdir = filepath.Join(r.cfg.Workspace, workdir)
			}
			workdir = filepath.Clean(workdir)
		}

		envMap, err := render.Map(step.Env, data)
		if err != nil {
			return fmt.Errorf("render env: %w", err)
		}

		if r.shouldLogProgress() {
			fmt.Fprintf(r.stdout, "\n--> exec step: %s\n", step.Name)
			fmt.Fprintf(r.stdout, "    %s\n", strings.Join(command, " "))
		}
		if r.cfg.DryRun {
			return nil
		}

		cmd := exec.CommandContext(stepCtx, command[0], command[1:]...)
		cmd.Dir = workdir
		cmd.Stdout = r.stdout
		cmd.Stderr = r.stderr
		cmd.Env = os.Environ()
		for key, value := range envMap {
			cmd.Env = append(cmd.Env, key+"="+value)
		}

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("run command: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported step type %q", step.Type)
	}
}

func (r *Runner) shouldLogProgress() bool {
	return r.cfg.DryRun || !r.agent.IsFullscreenTUI()
}
