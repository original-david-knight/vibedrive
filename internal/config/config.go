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
	Path                 string              `yaml:"-"`
	BaseDir              string              `yaml:"-"`
	DryRun               bool                `yaml:"dry_run"`
	Workspace            string              `yaml:"workspace"`
	TodoFile             string              `yaml:"todo_file"`
	PlanFile             string              `yaml:"plan_file"`
	MaxIterations        int                 `yaml:"max_iterations"`
	MaxStalledIterations int                 `yaml:"max_stalled_iterations"`
	DefaultWorkflow      string              `yaml:"default_workflow"`
	Claude               ClaudeConfig        `yaml:"claude"`
	Steps                []Step              `yaml:"steps"`
	Workflows            map[string]Workflow `yaml:"workflows"`
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
	FreshSession    bool              `yaml:"fresh_session"`
	Timeout         string            `yaml:"timeout"`
	ContinueOnError bool              `yaml:"continue_on_error"`
	Disabled        bool              `yaml:"disabled"`
}

type Workflow struct {
	Steps []Step `yaml:"steps"`
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

	if cfg.PlanFile != "" {
		if !filepath.IsAbs(cfg.PlanFile) {
			cfg.PlanFile = filepath.Join(cfg.Workspace, cfg.PlanFile)
		}
		cfg.PlanFile = filepath.Clean(cfg.PlanFile)
	}

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
	for name, workflow := range cfg.Workflows {
		for i := range workflow.Steps {
			if workflow.Steps[i].Type == "" {
				workflow.Steps[i].Type = StepTypeClaude
			}
		}
		cfg.Workflows[name] = workflow
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

	if len(c.Steps) == 0 && len(c.Workflows) == 0 {
		return fmt.Errorf("at least one step or workflow is required")
	}

	for i, step := range c.Steps {
		if err := validateStep(fmt.Sprintf("steps[%d]", i), step); err != nil {
			return err
		}
	}

	for name, workflow := range c.Workflows {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("workflow names must not be empty")
		}
		if len(workflow.Steps) == 0 {
			return fmt.Errorf("workflows.%s.steps must not be empty", name)
		}
		for i, step := range workflow.Steps {
			if err := validateStep(fmt.Sprintf("workflows.%s.steps[%d]", name, i), step); err != nil {
				return err
			}
		}
	}

	if c.PlanFile != "" {
		if strings.TrimSpace(c.DefaultWorkflow) != "" {
			if _, ok := c.Workflows[c.DefaultWorkflow]; !ok && len(c.Steps) == 0 {
				return fmt.Errorf("default_workflow %q does not exist", c.DefaultWorkflow)
			}
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

func validateStep(path string, step Step) error {
	if step.Name == "" {
		return fmt.Errorf("%s.name is required", path)
	}

	switch normalize(step.Type) {
	case StepTypeClaude:
		if strings.TrimSpace(step.Prompt) == "" {
			return fmt.Errorf("%s.prompt is required for claude steps", path)
		}
	case StepTypeExec:
		if len(step.Command) == 0 {
			return fmt.Errorf("%s.command is required for exec steps", path)
		}
	default:
		return fmt.Errorf("%s.type %q is not supported", path, step.Type)
	}

	return nil
}
