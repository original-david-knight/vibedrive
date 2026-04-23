package bootstrap

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ghost_claude/internal/claude"
	"ghost_claude/internal/config"
)

type fakeClient struct {
	prompts []string
	closed  bool
}

func (f *fakeClient) RunPrompt(_ context.Context, _ *claude.Session, prompt string) error {
	f.prompts = append(f.prompts, prompt)
	return nil
}

func (f *fakeClient) Close(_ *claude.Session) error {
	f.closed = true
	return nil
}

func TestInitializerRunWritesConfigAndBootstrapsPlan(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")
	designPath := filepath.Join(dir, "DESIGN.md")

	if err := os.WriteFile(designPath, []byte("# Design\n\nproject constraints\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	client := &fakeClient{}
	init := New(io.Discard, io.Discard)
	init.newClient = func(cfg *config.Config, stdout, stderr io.Writer) (promptClient, error) {
		if cfg.PlanFile != filepath.Join(dir, "ghost-plan.yaml") {
			t.Fatalf("expected plan path to resolve under workspace, got %q", cfg.PlanFile)
		}
		return client, nil
	}
	init.newSession = func(strategy string) (*claude.Session, error) {
		return &claude.Session{Strategy: strategy, ID: "session-1"}, nil
	}

	if err := init.Run(context.Background(), configPath, []string{designPath}, false); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(client.prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(client.prompts))
	}
	if !strings.Contains(client.prompts[0], "Create ghost-plan.yaml") {
		t.Fatalf("expected first prompt to create the plan file, got %q", client.prompts[0])
	}
	if !strings.Contains(client.prompts[0], "what it learned in that phase") {
		t.Fatalf("expected first prompt to require per-task phase notes, got %q", client.prompts[0])
	}
	if !strings.Contains(client.prompts[0], "keep testing, verification, and cleanup work attached to the implementation task") {
		t.Fatalf("expected first prompt to keep testing and cleanup inline by default, got %q", client.prompts[0])
	}
	if !strings.Contains(client.prompts[0], "expected to introduce a new abstraction, risky temporary coupling or workaround, destructive or stateful behavior, or a broad expected implementation surface") {
		t.Fatalf("expected first prompt to describe trigger-based tech-debt rules, got %q", client.prompts[0])
	}
	if !strings.Contains(client.prompts[0], "do not claim the plan can know actual changed-file counts or other finalize-time facts before execution") {
		t.Fatalf("expected first prompt to distinguish planning heuristics from finalize-time facts, got %q", client.prompts[0])
	}
	if !strings.Contains(client.prompts[0], "do not add standalone tech-debt tasks on a fixed schedule") {
		t.Fatalf("expected first prompt to reject fixed tech-debt cadence, got %q", client.prompts[0])
	}
	if strings.Contains(client.prompts[0], "after every 5 significant dev steps") {
		t.Fatalf("expected first prompt to remove the old tech-debt cadence, got %q", client.prompts[0])
	}
	if strings.Contains(client.prompts[0], "Replace the file if it already exists.") {
		t.Fatalf("expected first prompt to omit replace instructions, got %q", client.prompts[0])
	}
	if !strings.Contains(client.prompts[0], "DESIGN.md") {
		t.Fatalf("expected first prompt to reference DESIGN.md, got %q", client.prompts[0])
	}
	if strings.Contains(client.prompts[0], "TODO.md") {
		t.Fatalf("expected first prompt to stop assuming TODO.md, got %q", client.prompts[0])
	}
	if !strings.Contains(client.prompts[1], "Perform a critical review of the plan") {
		t.Fatalf("expected second prompt to request a critical plan review, got %q", client.prompts[1])
	}
	if !strings.Contains(client.prompts[1], "capturing phase learnings") {
		t.Fatalf("expected second prompt to review note-capture coverage, got %q", client.prompts[1])
	}
	if !strings.Contains(client.prompts[1], "missing trigger-justified standalone tech-debt tasks") {
		t.Fatalf("expected second prompt to review trigger-based tech-debt gaps, got %q", client.prompts[1])
	}
	if !strings.Contains(client.prompts[1], "plan-time knowledge of actual changed-file counts or other finalize-time facts") {
		t.Fatalf("expected second prompt to review planning-boundary violations, got %q", client.prompts[1])
	}
	if !strings.Contains(client.prompts[1], "defer routine testing, verification, or cleanup work that should stay attached to implementation") {
		t.Fatalf("expected second prompt to keep routine testing and cleanup inline, got %q", client.prompts[1])
	}
	if strings.Contains(client.prompts[1], "required 2 tech-debt tasks after each block of 5 significant dev steps") {
		t.Fatalf("expected second prompt to remove the old tech-debt cadence review, got %q", client.prompts[1])
	}
	if strings.Contains(client.prompts[1], "/codex") {
		t.Fatalf("expected second prompt to stop requiring /codex, got %q", client.prompts[1])
	}
	if !client.closed {
		t.Fatal("expected client to be closed")
	}
}

func TestInitializerRunSkipsExistingPlanWithoutForce(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")
	planPath := filepath.Join(dir, "ghost-plan.yaml")
	sourcePath := filepath.Join(dir, "DESIGN.md")

	if err := os.WriteFile(planPath, []byte("existing plan\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("existing source\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	client := &fakeClient{}
	init := New(io.Discard, io.Discard)
	init.newClient = func(cfg *config.Config, stdout, stderr io.Writer) (promptClient, error) {
		return client, nil
	}

	if err := init.Run(context.Background(), configPath, []string{sourcePath}, false); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(client.prompts) != 0 {
		t.Fatalf("expected no prompts when plan already exists, got %d", len(client.prompts))
	}
}

func TestInitializerRunRegeneratesPlanWithForce(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")
	planPath := filepath.Join(dir, "ghost-plan.yaml")
	sourcePath := filepath.Join(dir, "DESIGN.md")

	if err := os.WriteFile(planPath, []byte("existing plan\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("existing source\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	client := &fakeClient{}
	init := New(io.Discard, io.Discard)
	init.newClient = func(cfg *config.Config, stdout, stderr io.Writer) (promptClient, error) {
		return client, nil
	}
	init.newSession = func(strategy string) (*claude.Session, error) {
		return &claude.Session{Strategy: strategy, ID: "session-1"}, nil
	}

	if err := init.Run(context.Background(), configPath, []string{sourcePath}, true); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(client.prompts) != 2 {
		t.Fatalf("expected forced init to regenerate the plan, got %d prompts", len(client.prompts))
	}
	if !strings.Contains(client.prompts[0], "ghost-plan.yaml") {
		t.Fatalf("expected first prompt to mention the plan path, got %q", client.prompts[0])
	}
	if _, err := os.Stat(planPath); !os.IsNotExist(err) {
		t.Fatalf("expected existing plan file to be removed before prompting, stat err=%v", err)
	}
}

func TestInitializerRunUsesWorkspaceFilesWhenSourceOmitted(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")

	if err := os.WriteFile(filepath.Join(dir, "DESIGN.md"), []byte("design\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TEST_PLAN.md"), []byte("tests\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	client := &fakeClient{}
	init := New(io.Discard, io.Discard)
	init.newClient = func(cfg *config.Config, stdout, stderr io.Writer) (promptClient, error) {
		return client, nil
	}
	init.newSession = func(strategy string) (*claude.Session, error) {
		return &claude.Session{Strategy: strategy, ID: "session-1"}, nil
	}

	if err := init.Run(context.Background(), configPath, nil, false); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(client.prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(client.prompts))
	}
	if !strings.Contains(client.prompts[0], "- DESIGN.md") {
		t.Fatalf("expected first prompt to include DESIGN.md as a source, got %q", client.prompts[0])
	}
	if !strings.Contains(client.prompts[0], "- TEST_PLAN.md") {
		t.Fatalf("expected first prompt to include TEST_PLAN.md as a source, got %q", client.prompts[0])
	}
	if strings.Contains(client.prompts[0], "- ghost-claude.yaml") {
		t.Fatalf("expected generated config to be excluded from default sources, got %q", client.prompts[0])
	}
}

func TestInitializerRunRendersResolvedSourcesInSortedOrder(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")
	docsDir := filepath.Join(dir, "docs")

	if err := os.Mkdir(docsDir, 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "zeta.md"), []byte("zeta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "alpha.md"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	client := &fakeClient{}
	init := New(io.Discard, io.Discard)
	init.newClient = func(cfg *config.Config, stdout, stderr io.Writer) (promptClient, error) {
		return client, nil
	}
	init.newSession = func(strategy string) (*claude.Session, error) {
		return &claude.Session{Strategy: strategy, ID: "session-1"}, nil
	}

	if err := init.Run(context.Background(), configPath, []string{"docs/zeta.md", "docs"}, false); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if len(client.prompts) != 2 {
		t.Fatalf("expected 2 prompts, got %d", len(client.prompts))
	}

	alphaIndex := strings.Index(client.prompts[0], "- docs/alpha.md")
	zetaIndex := strings.Index(client.prompts[0], "- docs/zeta.md")
	if alphaIndex == -1 || zetaIndex == -1 {
		t.Fatalf("expected prompt to include both resolved sources, got %q", client.prompts[0])
	}
	if alphaIndex > zetaIndex {
		t.Fatalf("expected prompt to render sources in sorted order, got %q", client.prompts[0])
	}
}

func TestInitializerPrintSourcesResolvesPreviewWithoutWritingConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")
	planPath := filepath.Join(dir, "ghost-plan.yaml")

	if err := os.WriteFile(filepath.Join(dir, "DESIGN.md"), []byte("design\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "TEST_PLAN.md"), []byte("tests\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(planPath, []byte("existing plan\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	var stdout bytes.Buffer
	init := New(&stdout, io.Discard)

	if err := init.PrintSources(configPath, nil); err != nil {
		t.Fatalf("PrintSources returned error: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "- DESIGN.md") {
		t.Fatalf("expected preview output to include DESIGN.md, got %q", output)
	}
	if !strings.Contains(output, "- TEST_PLAN.md") {
		t.Fatalf("expected preview output to include TEST_PLAN.md, got %q", output)
	}
	if strings.Contains(output, "- ghost-plan.yaml") {
		t.Fatalf("expected preview output to exclude the plan file, got %q", output)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Fatalf("expected PrintSources not to write config, stat err=%v", err)
	}
}

func TestResolveSourcesDedupesAndSortsResolvedFiles(t *testing.T) {
	dir := t.TempDir()
	docsDir := filepath.Join(dir, "docs")

	if err := os.Mkdir(docsDir, 0o755); err != nil {
		t.Fatalf("Mkdir returned error: %v", err)
	}
	alphaPath := filepath.Join(docsDir, "alpha.md")
	betaPath := filepath.Join(docsDir, "beta.md")
	if err := os.WriteFile(alphaPath, []byte("alpha\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(betaPath, []byte("beta\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	got, err := resolveSources(dir, []string{"docs/beta.md", "docs"})
	if err != nil {
		t.Fatalf("resolveSources returned error: %v", err)
	}

	if len(got.Files) != 2 {
		t.Fatalf("expected 2 unique resolved files, got %d", len(got.Files))
	}
	if got.Files[0] != alphaPath || got.Files[1] != betaPath {
		t.Fatalf("expected sorted files [%q %q], got %v", alphaPath, betaPath, got.Files)
	}
}

func TestResolveSourcesRejectsEmptySelection(t *testing.T) {
	if _, err := resolveSources(t.TempDir(), []string{"   "}); err == nil {
		t.Fatal("expected resolveSources to reject an empty explicit source")
	}
}

func TestResolveSourcesRejectsEmptyDirectorySelection(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")
	planPath := filepath.Join(dir, "ghost-plan.yaml")

	if err := os.WriteFile(configPath, []byte("config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(planPath, []byte("plan\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := resolveSources(dir, nil, configPath, planPath); err == nil {
		t.Fatal("expected resolveSources to reject a directory with no usable regular files")
	}
}
