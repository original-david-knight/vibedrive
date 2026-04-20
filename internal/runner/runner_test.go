package runner

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ghost_claude/internal/claude"
	"ghost_claude/internal/config"
	"ghost_claude/internal/plan"
)

type fakeAgent struct {
	prompts         []string
	sessionIDs      []string
	closedSessionID []string
	todoPath        string
	planPath        string
}

func (f *fakeAgent) RunPrompt(_ context.Context, session *claude.Session, prompt string) error {
	f.prompts = append(f.prompts, prompt)
	f.sessionIDs = append(f.sessionIDs, session.ID)

	if strings.HasPrefix(prompt, "mark ") {
		return markFirstIncompleteTodoDone(f.todoPath)
	}
	if strings.HasPrefix(prompt, "finish task ") {
		taskID := strings.TrimPrefix(prompt, "finish task ")
		return updateTask(f.planPath, taskID, plan.StatusDone, "done")
	}
	if strings.HasPrefix(prompt, "progress task ") {
		taskID := strings.TrimPrefix(prompt, "progress task ")
		return updateTask(f.planPath, taskID, plan.StatusInProgress, "still working")
	}

	return nil
}

func (f *fakeAgent) Close(session *claude.Session) error {
	f.closedSessionID = append(f.closedSessionID, session.ID)
	return nil
}

func (f *fakeAgent) IsFullscreenTUI() bool {
	return false
}

func TestRunCreatesFreshClaudeSessionPerTodo(t *testing.T) {
	dir := t.TempDir()
	todoPath := filepath.Join(dir, "TODO.md")

	content := `# TODO

- [ ] first item
- [ ] second item
`
	if err := os.WriteFile(todoPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg := &config.Config{
		Path:                 filepath.Join(dir, "ghost-claude.yaml"),
		Workspace:            dir,
		TodoFile:             todoPath,
		MaxStalledIterations: 1,
		Claude: config.ClaudeConfig{
			SessionStrategy: config.SessionStrategySessionID,
		},
		Steps: []config.Step{
			{Name: "analyze", Type: config.StepTypeClaude, Prompt: "analyze {{ .NextTodo.Text }}"},
			{Name: "mark", Type: config.StepTypeClaude, Prompt: "mark {{ .NextTodo.Text }}"},
		},
	}

	agent := &fakeAgent{todoPath: todoPath}
	sessionCount := 0

	r := &Runner{
		cfg:    cfg,
		stdout: io.Discard,
		stderr: io.Discard,
		agent:  agent,
		newSession: func(_ string) (*claude.Session, error) {
			sessionCount++
			return &claude.Session{
				Strategy: config.SessionStrategySessionID,
				ID:       "session-" + string(rune('0'+sessionCount)),
			}, nil
		},
	}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if sessionCount != 2 {
		t.Fatalf("expected 2 sessions, got %d", sessionCount)
	}

	wantPromptSessions := []string{"session-1", "session-1", "session-2", "session-2"}
	if strings.Join(agent.sessionIDs, ",") != strings.Join(wantPromptSessions, ",") {
		t.Fatalf("expected prompt session IDs %v, got %v", wantPromptSessions, agent.sessionIDs)
	}

	wantClosedSessions := []string{"session-1", "session-2"}
	if strings.Join(agent.closedSessionID, ",") != strings.Join(wantClosedSessions, ",") {
		t.Fatalf("expected closed session IDs %v, got %v", wantClosedSessions, agent.closedSessionID)
	}
}

func TestRunCreatesFreshClaudeSessionPerMarkedStep(t *testing.T) {
	dir := t.TempDir()
	todoPath := filepath.Join(dir, "TODO.md")

	content := `# TODO

- [ ] first item
`
	if err := os.WriteFile(todoPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg := &config.Config{
		Path:                 filepath.Join(dir, "ghost-claude.yaml"),
		Workspace:            dir,
		TodoFile:             todoPath,
		MaxStalledIterations: 1,
		Claude: config.ClaudeConfig{
			SessionStrategy: config.SessionStrategySessionID,
		},
		Steps: []config.Step{
			{Name: "analyze", Type: config.StepTypeClaude, Prompt: "analyze {{ .NextTodo.Text }}"},
			{Name: "mark", Type: config.StepTypeClaude, Prompt: "mark {{ .NextTodo.Text }}", FreshSession: true},
		},
	}

	agent := &fakeAgent{todoPath: todoPath}
	sessionCount := 0

	r := &Runner{
		cfg:    cfg,
		stdout: io.Discard,
		stderr: io.Discard,
		agent:  agent,
		newSession: func(_ string) (*claude.Session, error) {
			sessionCount++
			return &claude.Session{
				Strategy: config.SessionStrategySessionID,
				ID:       "session-" + string(rune('0'+sessionCount)),
			}, nil
		},
	}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if sessionCount != 2 {
		t.Fatalf("expected 2 sessions, got %d", sessionCount)
	}

	wantPromptSessions := []string{"session-1", "session-2"}
	if strings.Join(agent.sessionIDs, ",") != strings.Join(wantPromptSessions, ",") {
		t.Fatalf("expected prompt session IDs %v, got %v", wantPromptSessions, agent.sessionIDs)
	}
}

func TestRunExplainsStalledTodoProgress(t *testing.T) {
	dir := t.TempDir()
	todoPath := filepath.Join(dir, "TODO.md")

	content := `# TODO

- [ ] first item
`
	if err := os.WriteFile(todoPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg := &config.Config{
		Path:                 filepath.Join(dir, "ghost-claude.yaml"),
		Workspace:            dir,
		TodoFile:             todoPath,
		MaxStalledIterations: 1,
		Claude: config.ClaudeConfig{
			SessionStrategy: config.SessionStrategySessionID,
		},
		Steps: []config.Step{
			{Name: "analyze", Type: config.StepTypeClaude, Prompt: "analyze {{ .NextTodo.Text }}"},
		},
	}

	r := &Runner{
		cfg:    cfg,
		stdout: io.Discard,
		stderr: io.Discard,
		agent:  &fakeAgent{todoPath: todoPath},
		newSession: func(_ string) (*claude.Session, error) {
			return &claude.Session{
				Strategy: config.SessionStrategySessionID,
				ID:       "session-1",
			}, nil
		},
	}

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected Run to fail when the TODO file does not change")
	}

	message := err.Error()
	if !strings.Contains(message, "ghost-claude only advances when the first incomplete checkbox changes") {
		t.Fatalf("expected stall error to explain TODO progress detection, got %q", message)
	}
	if !strings.Contains(message, "Raise max_stalled_iterations if you want automatic retries") {
		t.Fatalf("expected stall error to suggest retries, got %q", message)
	}
}

func TestRunExecutesReadyPlanTasksByDependencyOrder(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "ghost-plan.yaml")

	content := `project:
  name: demo
tasks:
  - id: scaffold
    title: Scaffold repo
    workflow: implement
    status: todo
  - id: inspect
    title: Implement inspect
    workflow: implement
    status: todo
    deps:
      - scaffold
`
	if err := os.WriteFile(planPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg := &config.Config{
		Path:                 filepath.Join(dir, "ghost-claude.yaml"),
		Workspace:            dir,
		PlanFile:             planPath,
		MaxStalledIterations: 1,
		Claude: config.ClaudeConfig{
			SessionStrategy: config.SessionStrategySessionID,
		},
		DefaultWorkflow: "implement",
		Workflows: map[string]config.Workflow{
			"implement": {
				Steps: []config.Step{
					{Name: "analyze", Type: config.StepTypeClaude, Prompt: "analyze {{ .Task.ID }}"},
					{Name: "finish", Type: config.StepTypeClaude, Prompt: "finish task {{ .Task.ID }}"},
				},
			},
		},
	}

	agent := &fakeAgent{planPath: planPath}
	sessionCount := 0
	r := &Runner{
		cfg:    cfg,
		stdout: io.Discard,
		stderr: io.Discard,
		agent:  agent,
		newSession: func(_ string) (*claude.Session, error) {
			sessionCount++
			return &claude.Session{
				Strategy: config.SessionStrategySessionID,
				ID:       "session-" + string(rune('0'+sessionCount)),
			}, nil
		},
	}

	if err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if sessionCount != 2 {
		t.Fatalf("expected 2 sessions, got %d", sessionCount)
	}

	loaded, err := plan.Load(planPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	for _, task := range loaded.Tasks {
		if task.Status != plan.StatusDone {
			t.Fatalf("expected task %q to be done, got %q", task.ID, task.Status)
		}
	}

	wantPrompts := []string{
		"analyze scaffold",
		"finish task scaffold",
		"analyze inspect",
		"finish task inspect",
	}
	if strings.Join(agent.prompts, "\n") != strings.Join(wantPrompts, "\n") {
		t.Fatalf("unexpected prompts:\n%s", strings.Join(agent.prompts, "\n"))
	}
}

func TestRunExplainsStalledPlanProgress(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "ghost-plan.yaml")

	content := `project:
  name: demo
tasks:
  - id: scaffold
    title: Scaffold repo
    workflow: implement
    status: todo
`
	if err := os.WriteFile(planPath, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	cfg := &config.Config{
		Path:                 filepath.Join(dir, "ghost-claude.yaml"),
		Workspace:            dir,
		PlanFile:             planPath,
		MaxStalledIterations: 1,
		Claude: config.ClaudeConfig{
			SessionStrategy: config.SessionStrategySessionID,
		},
		DefaultWorkflow: "implement",
		Workflows: map[string]config.Workflow{
			"implement": {
				Steps: []config.Step{
					{Name: "analyze", Type: config.StepTypeClaude, Prompt: "analyze {{ .Task.ID }}"},
				},
			},
		},
	}

	r := &Runner{
		cfg:    cfg,
		stdout: io.Discard,
		stderr: io.Discard,
		agent:  &fakeAgent{planPath: planPath},
		newSession: func(_ string) (*claude.Session, error) {
			return &claude.Session{
				Strategy: config.SessionStrategySessionID,
				ID:       "session-1",
			}, nil
		},
	}

	err := r.Run(context.Background())
	if err == nil {
		t.Fatal("expected Run to fail when the plan task does not change")
	}

	message := err.Error()
	if !strings.Contains(message, "made no task progress") {
		t.Fatalf("expected plan stall explanation, got %q", message)
	}
	if !strings.Contains(message, "status") {
		t.Fatalf("expected plan stall error to mention status, got %q", message)
	}
}

func markFirstIncompleteTodoDone(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	updated := strings.Replace(string(data), "- [ ]", "- [x]", 1)
	return os.WriteFile(path, []byte(updated), 0o644)
}

func updateTask(path, taskID, status, notes string) error {
	file, err := plan.Load(path)
	if err != nil {
		return err
	}

	for i := range file.Tasks {
		if file.Tasks[i].ID == taskID {
			file.Tasks[i].Status = status
			file.Tasks[i].Notes = notes
			return file.Save()
		}
	}

	return os.ErrNotExist
}
