package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	StepTypeClaude = "claude"
	StepTypeExec   = "exec"

	ClaudeTransportPrint = "print"
	ClaudeTransportTUI   = "tui"

	SessionStrategySessionID = "session_id"
	SessionStrategyContinue  = "continue"
)

type Config struct {
	Path                 string       `yaml:"-"`
	BaseDir              string       `yaml:"-"`
	DryRun               bool         `yaml:"dry_run"`
	Workspace            string       `yaml:"workspace"`
	TodoFile             string       `yaml:"todo_file"`
	MaxIterations        int          `yaml:"max_iterations"`
	MaxStalledIterations int          `yaml:"max_stalled_iterations"`
	Claude               ClaudeConfig `yaml:"claude"`
	Steps                []Step       `yaml:"steps"`
}

type ClaudeConfig struct {
	Command         string   `yaml:"command"`
	Args            []string `yaml:"args"`
	Transport       string   `yaml:"transport"`
	StartupTimeout  string   `yaml:"startup_timeout"`
	SessionStrategy string   `yaml:"session_strategy"`
}

type Step struct {
	Name            string            `yaml:"name"`
	Type            string            `yaml:"type"`
	Prompt          string            `yaml:"prompt"`
	Command         []string          `yaml:"command"`
	WorkingDir      string            `yaml:"working_dir"`
	Env             map[string]string `yaml:"env"`
	Timeout         string            `yaml:"timeout"`
	ContinueOnError bool              `yaml:"continue_on_error"`
	Disabled        bool              `yaml:"disabled"`
}

func Load(path string) (*Config, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	cfg := defaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	cfg.Path = absPath
	cfg.BaseDir = filepath.Dir(absPath)

	if cfg.Workspace == "" {
		cfg.Workspace = "."
	}
	if !filepath.IsAbs(cfg.Workspace) {
		cfg.Workspace = filepath.Join(cfg.BaseDir, cfg.Workspace)
	}
	cfg.Workspace = filepath.Clean(cfg.Workspace)

	if cfg.TodoFile == "" {
		cfg.TodoFile = "TODO.md"
	}
	if !filepath.IsAbs(cfg.TodoFile) {
		cfg.TodoFile = filepath.Join(cfg.Workspace, cfg.TodoFile)
	}
	cfg.TodoFile = filepath.Clean(cfg.TodoFile)

	if cfg.Claude.Command == "" {
		cfg.Claude.Command = "claude"
	}
	if cfg.Claude.Transport == "" {
		cfg.Claude.Transport = ClaudeTransportTUI
	}
	if cfg.Claude.StartupTimeout == "" {
		cfg.Claude.StartupTimeout = "30s"
	}
	if cfg.Claude.SessionStrategy == "" {
		cfg.Claude.SessionStrategy = SessionStrategySessionID
	}

	for i := range cfg.Steps {
		if cfg.Steps[i].Type == "" {
			cfg.Steps[i].Type = StepTypeClaude
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.MaxIterations < 0 {
		return fmt.Errorf("max_iterations must be >= 0")
	}
	if c.MaxStalledIterations < 1 {
		return fmt.Errorf("max_stalled_iterations must be >= 1")
	}
	if c.Claude.Command == "" {
		return fmt.Errorf("claude.command is required")
	}

	switch normalize(c.Claude.Transport) {
	case ClaudeTransportPrint, ClaudeTransportTUI:
	default:
		return fmt.Errorf("unsupported claude.transport %q", c.Claude.Transport)
	}

	if strings.TrimSpace(c.Claude.StartupTimeout) == "" {
		return fmt.Errorf("claude.startup_timeout is required")
	}

	switch normalize(c.Claude.SessionStrategy) {
	case "", SessionStrategySessionID, SessionStrategyContinue:
	default:
		return fmt.Errorf("unsupported claude.session_strategy %q", c.Claude.SessionStrategy)
	}

	if len(c.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}

	for i, step := range c.Steps {
		if step.Name == "" {
			return fmt.Errorf("steps[%d].name is required", i)
		}

		switch normalize(step.Type) {
		case StepTypeClaude:
			if strings.TrimSpace(step.Prompt) == "" {
				return fmt.Errorf("steps[%d].prompt is required for claude steps", i)
			}
		case StepTypeExec:
			if len(step.Command) == 0 {
				return fmt.Errorf("steps[%d].command is required for exec steps", i)
			}
		default:
			return fmt.Errorf("steps[%d].type %q is not supported", i, step.Type)
		}
	}

	return nil
}

func defaultConfig() Config {
	return Config{
		Workspace:            ".",
		TodoFile:             "TODO.md",
		MaxStalledIterations: 2,
		Claude: ClaudeConfig{
			Command:         "claude",
			Transport:       ClaudeTransportTUI,
			StartupTimeout:  "30s",
			SessionStrategy: SessionStrategySessionID,
		},
	}
}

func normalize(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}
