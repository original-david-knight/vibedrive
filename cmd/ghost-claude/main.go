package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"ghost_claude/internal/config"
	"ghost_claude/internal/runner"
	"ghost_claude/internal/scaffold"
)

func main() {
	if err := realMain(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func realMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := os.Args[1:]
	if len(args) == 0 {
		return runCommand(ctx, []string{})
	}

	switch args[0] {
	case "run":
		return runCommand(ctx, args[1:])
	case "init":
		return initCommand(args[1:])
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return runCommand(ctx, args)
	}
}

func runCommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	configPath := fs.String("config", "ghost-claude.yaml", "Path to the workflow config file")
	workspace := fs.String("workspace", "", "Workspace directory containing the workflow config")
	dryRun := fs.Bool("dry-run", false, "Render prompts and commands without executing them")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	resolvedConfigPath, err := resolveConfigPath(*configPath, *workspace)
	if err != nil {
		return err
	}

	cfg, err := config.Load(resolvedConfigPath)
	if err != nil {
		return err
	}
	cfg.DryRun = cfg.DryRun || *dryRun

	app, err := runner.New(cfg, os.Stdout, os.Stderr)
	if err != nil {
		return err
	}
	return app.Run(ctx)
}

func initCommand(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	configPath := fs.String("config", "ghost-claude.yaml", "Path to write the workflow config file")
	workspace := fs.String("workspace", "", "Workspace directory where the config and TODO file should be created")
	force := fs.Bool("force", false, "Overwrite existing files")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	absConfig, err := resolveConfigPath(*configPath, *workspace)
	if err != nil {
		return err
	}

	return scaffold.Write(absConfig, *force)
}

func printUsage() {
	fmt.Println(`ghost-claude

Usage:
  ghost-claude run [-config ghost-claude.yaml] [-workspace /path/to/repo] [-dry-run]
  ghost-claude init [-config ghost-claude.yaml] [-workspace /path/to/repo] [-force]

If no subcommand is provided, ghost-claude behaves like "run".`)
}

func resolveConfigPath(configPath, workspace string) (string, error) {
	if filepath.IsAbs(configPath) {
		return filepath.Clean(configPath), nil
	}

	if workspace == "" {
		return filepath.Abs(configPath)
	}

	absWorkspace, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}

	return filepath.Join(absWorkspace, configPath), nil
}
