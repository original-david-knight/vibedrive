# ghost-claude

`ghost-claude` is a terminal-native Go runner that drives Claude Code through a configurable workflow. It supports two execution modes:

- TODO mode: walk the first unchecked item in `TODO.md`
- plan mode: execute tasks from a machine-readable `ghost-plan.yaml`

It launches Claude's real fullscreen TUI inside a PTY, types prompts as terminal input, and waits for Claude to return to idle before sending the next step. Each work item gets a fresh Claude session, and individual Claude steps can opt into their own fresh sessions too.

## Requirements

- Go 1.22+
- The `claude` CLI installed and on your `$PATH` ([Claude Code](https://docs.claude.com/en/docs/claude-code))
- Either a `TODO.md` file with GitHub-style checkboxes or a `ghost-plan.yaml` file with machine-readable tasks

## Install

```bash
go install ./cmd/ghost-claude
```

Or run without installing:

```bash
go run ./cmd/ghost-claude <subcommand>
```

## Quick start

From inside the repo you want ghost-claude to work on:

```bash
ghost-claude init              # writes ghost-claude.yaml, uses all regular files in the workspace dir as source, then asks Claude to generate ghost-plan.yaml and review it with Codex
ghost-claude init DESIGN.md    # or point init at a specific source file or directory
ghost-claude run    # starts the loop
```

Target a different repo without `cd`:

```bash
ghost-claude init --workspace /path/to/repo
ghost-claude run  --workspace /path/to/repo
```

Preview what would happen without touching anything:

```bash
ghost-claude run --dry-run
```

## How the loop works

For each iteration:

1. Select the next work item.
   In TODO mode this is the first unchecked `- [ ]` item in `TODO.md`.
   In plan mode this is the first ready task in `ghost-plan.yaml`, with `in_progress` tasks preferred over `todo` tasks and dependencies respected.
2. Start a fresh Claude session when a Claude step needs one.
3. Run every configured step in order. By default Claude steps share one session for the work item, but `fresh_session: true` isolates a step in its own Claude session.
4. Close any Claude sessions that were opened.
5. Re-read the queue state. If the selected item changed state, advance. If not, count a stall and retry.

The runner stops when there is no work left, when `max_iterations` is reached, or when the same item stalls `max_stalled_iterations` times in a row.

The default workflow scaffolded by `ghost-claude init` is plan-oriented:

1. Execute the selected task in a fresh Claude session while preserving the plan's hard constraints.
2. Ask Codex for a review on non-trivial diffs in a separate fresh Claude session.
3. Run the task's configured `verify_commands`, apply the JSON task result to `ghost-plan.yaml`, and commit the iteration with an exec step.

During `init`, ghost-claude bootstraps plan mode in two phases:

1. Write `ghost-claude.yaml`.
2. Ask Claude to read the provided source file or directory, or all regular files in the workspace directory when no source is provided, then generate `ghost-plan.yaml`, perform a Codex-backed review, and revise the plan.

## Subcommands

```
ghost-claude run  [-config PATH] [-workspace DIR] [-dry-run]
ghost-claude init [-config PATH] [-workspace DIR] [-source PATH] [-force] [SOURCE]
ghost-claude task finalize --workspace DIR --plan PATH --task TASK_ID --result PATH [--message MSG]
ghost-claude help
```

If you omit the subcommand, `ghost-claude` behaves like `run`.

When `-workspace` is set, the default config path becomes `<workspace>/ghost-claude.yaml` and every relative path in the config resolves against that workspace.

## Config

The runner reads `ghost-claude.yaml` by default. Recommended plan-mode example:

```yaml
workspace: .
plan_file: ghost-plan.yaml
max_iterations: 0
max_stalled_iterations: 2
default_workflow: implement

claude:
  command: claude
  transport: tui
  startup_timeout: 30s
  session_strategy: session_id
  args:
    - --permission-mode
    - bypassPermissions

workflows:
  implement:
    steps:
      - name: execute-task
        type: claude
        fresh_session: true
        prompt: |
          Execute task {{ .Task.ID }} from {{ .PlanFile }} while preserving:
          {{- range .Plan.Project.ConstraintFiles }}
          - {{ . }}
          {{- end }}

      - name: finalize-task
        type: exec
        command:
          - "{{ .ExecutablePath }}"
          - task
          - finalize
          - --workspace
          - "{{ .Workspace }}"
          - --plan
          - "{{ .PlanFile }}"
          - --task
          - "{{ .Task.ID }}"
          - --result
          - "{{ .TaskResultPath }}"
```

Legacy TODO mode still works: omit `plan_file` and `workflows`, keep `steps`, and drive progress by editing the first unchecked checkbox in `TODO.md`.

### `ghost-plan.yaml`

Plan mode uses a machine-readable task file. Minimal example:

```yaml
project:
  name: planet-v1
  objective: Ship Planet v1 end-to-end in this repository.
  source_docs:
    - DESIGN.md
    - TEST_PLAN.md
  constraint_files:
    - DESIGN.md

tasks:
  - id: scaffold
    title: Scaffold the repo and get `go build ./...` green
    workflow: implement
    status: todo
    acceptance:
      - `go build ./...` succeeds
      - fast test script exists and runs
    verify_commands:
      - go build ./...

  - id: scaffold-checkpoint
    title: Run the first full-suite checkpoint and fix regressions
    workflow: checkpoint
    status: todo
    deps:
      - scaffold
    verify_commands:
      - go test ./...
```

The intended use is:

- `ghost-plan.yaml` is machine-owned execution state
- `TODO.md` is still useful when you want legacy checklist mode or want to keep one constraints doc
- `ghost-claude init` can generate the initial plan from a TODO file, a design doc, or a directory of source files
- your external planner can still generate both files if you prefer that flow

### Top-level fields

| Field                    | Default       | Meaning                                                            |
| ------------------------ | ------------- | ------------------------------------------------------------------ |
| `workspace`              | `.`           | Directory Claude runs in. Relative paths resolve from the config.  |
| `todo_file`              | `TODO.md`     | Legacy checklist file. Also useful as a constraints source.        |
| `plan_file`              | unset         | Machine-readable task file for plan mode.                          |
| `max_iterations`         | `0`           | Hard cap on iterations. `0` means unlimited.                       |
| `max_stalled_iterations` | `2`           | Abort after this many no-progress iterations on the same item.     |
| `default_workflow`       | unset         | Workflow used when a plan task omits `workflow`.                   |
| `dry_run`                | `false`       | Render prompts and commands without running anything.              |

### `claude` block

| Field              | Default      | Meaning                                                                 |
| ------------------ | ------------ | ----------------------------------------------------------------------- |
| `command`          | `claude`     | Executable to launch.                                                   |
| `args`             | `[]`         | Extra CLI flags passed to Claude.                                       |
| `transport`        | `tui`        | `tui` drives the fullscreen UI inside a PTY. `print` uses `--print`.   |
| `startup_timeout`  | `30s`        | How long to wait for Claude to become ready before failing.             |
| `session_strategy` | `session_id` | `session_id` starts a new session per item; `continue` resumes.         |

### Step fields

| Field               | Applies to  | Meaning                                                           |
| ------------------- | ----------- | ----------------------------------------------------------------- |
| `name`              | all         | Required. Shown in logs.                                          |
| `type`              | all         | `claude` (default) or `exec`.                                     |
| `prompt`            | claude      | Go template rendered and sent to Claude.                          |
| `command`           | exec        | Argv list to run. Each element is a Go template.                  |
| `working_dir`       | exec        | Defaults to `workspace`. Relative paths resolve from `workspace`. |
| `env`               | exec        | Extra env vars. Values are Go templates.                          |
| `fresh_session`     | claude      | Run this Claude step in a one-off session instead of the shared item session. |
| `timeout`           | all         | Go duration (for example `10m`). No timeout by default.           |
| `continue_on_error` | all         | Log the failure and keep going instead of aborting.               |
| `disabled`          | all         | Skip the step.                                                    |

### Workflow fields

| Field   | Meaning                           |
| ------- | --------------------------------- |
| `steps` | Ordered list of steps to execute. |

### Template data

Prompts, `command`, `working_dir`, and `env` values are rendered with Go's `text/template`. Available fields:

- `{{ .Workspace }}`
- `{{ .TodoFile }}`
- `{{ .PlanFile }}`
- `{{ .ConfigPath }}`
- `{{ .ExecutablePath }}`
- `{{ .Iteration }}`
- `{{ .SessionID }}`
- `{{ .TaskResultPath }}`
- `{{ .NextTodo.Line }}` — 1-indexed line number in `TODO.md`
- `{{ .NextTodo.Raw }}` — the entire line, including the checkbox
- `{{ .NextTodo.Text }}` — just the task description
- `{{ .Plan }}` — parsed `ghost-plan.yaml`
- `{{ .Task }}` — selected plan task in plan mode
- `{{ .Now }}` — current time

## Notes & gotchas

- In TODO mode, the runner only advances when the first incomplete checkbox in the TODO file changes.
- In plan mode, the runner only advances when the selected task changes status or notes in `ghost-plan.yaml`.
- `ghost-plan.yaml` is intended to be machine-owned state. The default workflow updates it through `ghost-claude task finalize`.
- `verify_commands` lets plan tasks declare deterministic checks for the exec finalizer to run before a task can stay `done`.
- Review prompts can assume Claude has a `/codex` command available and can use it for code or plan review.
- `ghost-claude init` uses the configured Claude transport. With the default `tui` transport, you can watch Claude generate and review the plan live in your terminal.
- In TUI mode, YAML multiline prompts are flattened into one submitted message, because real newlines would be interpreted as separate messages by Claude's composer.
- In a fresh workspace, the runner auto-confirms Claude's trust dialog so the loop can start unattended.
- TUI automation detects "Claude is idle" from terminal-title transitions. If a future Claude release changes that behavior, the detector may need updating.
- `type: exec` lets you move deterministic steps (linters, formatters, arbitrary shell) out of Claude when that becomes cleaner.
