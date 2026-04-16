package main

import (
	"path/filepath"
	"testing"
)

func TestResolveConfigPathWithoutWorkspace(t *testing.T) {
	got, err := resolveConfigPath("ghost-claude.yaml", "")
	if err != nil {
		t.Fatalf("resolveConfigPath returned error: %v", err)
	}

	want, err := filepath.Abs("ghost-claude.yaml")
	if err != nil {
		t.Fatalf("filepath.Abs returned error: %v", err)
	}

	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveConfigPathWithWorkspace(t *testing.T) {
	dir := t.TempDir()

	got, err := resolveConfigPath("ghost-claude.yaml", dir)
	if err != nil {
		t.Fatalf("resolveConfigPath returned error: %v", err)
	}

	want := filepath.Join(dir, "ghost-claude.yaml")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveConfigPathKeepsAbsoluteConfigPath(t *testing.T) {
	absConfig := filepath.Join(t.TempDir(), "ghost-claude.yaml")

	got, err := resolveConfigPath(absConfig, t.TempDir())
	if err != nil {
		t.Fatalf("resolveConfigPath returned error: %v", err)
	}

	if got != absConfig {
		t.Fatalf("expected %q, got %q", absConfig, got)
	}
}
