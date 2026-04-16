package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWritePreservesExistingTODO(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")
	todoPath := filepath.Join(dir, "TODO.md")

	if err := os.WriteFile(todoPath, []byte("existing todo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := Write(configPath, false); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(configContent), "workspace: .") {
		t.Fatalf("expected sample config content, got %q", string(configContent))
	}

	content, err := os.ReadFile(todoPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(content) != "existing todo\n" {
		t.Fatalf("expected existing TODO to be preserved, got %q", string(content))
	}
}

func TestWriteFailsWhenConfigExists(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")

	if err := os.WriteFile(configPath, []byte("old config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	err := Write(configPath, false)
	if err == nil {
		t.Fatal("expected Write to fail when config already exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already exists error, got %v", err)
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if string(content) != "old config\n" {
		t.Fatalf("expected existing config to be preserved, got %q", string(content))
	}
}

func TestWriteOverwritesWhenForceIsSet(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "ghost-claude.yaml")
	todoPath := filepath.Join(dir, "TODO.md")

	if err := os.WriteFile(configPath, []byte("old config\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	if err := os.WriteFile(todoPath, []byte("old todo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if err := Write(configPath, true); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(configContent), "workspace: .") {
		t.Fatalf("expected sample config content, got %q", string(configContent))
	}

	todoContent, err := os.ReadFile(todoPath)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !strings.Contains(string(todoContent), "- [ ] Replace this item with a real task.") {
		t.Fatalf("expected sample TODO content, got %q", string(todoContent))
	}
}
