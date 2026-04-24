package main

import (
	"context"
	"strings"
	"testing"
)

func TestRestartCommandRejectsPositionalArgs(t *testing.T) {
	err := restartCommand(context.Background(), []string{"extra"})
	if err == nil {
		t.Fatal("expected restartCommand to reject positional arguments")
	}
	if !strings.Contains(err.Error(), "does not accept positional arguments") {
		t.Fatalf("expected positional-argument error, got %v", err)
	}
}
