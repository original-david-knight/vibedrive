# ghost-claude

`ghost-claude` is a terminal-native Go runner that walks a `TODO.md` file and drives Claude Code through a configurable, multi-step workflow for each item.

It launches Claude's real fullscreen TUI inside a PTY, types prompts as terminal input, and waits for Claude to return to idle before sending the next step. Each TODO item gets a fresh Claude session, so nothing leaks from one item to the next. The loop keeps going until the TODO file is empty (or the same item fails to make progress twice in a row).

## Requirements

- Go 1.22+
- The `claude` CLI installed and on your `$PATH` ([Claude Code](https://docs.claude.com/en/docs/claude-code))
- A `TODO.md` file with GitHub-style checkboxes (`- [ ] ...`)

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
ghost-claude init   # writes ghost-claude.yaml with a sensible default workflow
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

1. Find the first unchecked `- [ ]` item in `TODO.md`.
2. Start a fresh Claude session.
3. Run every configured step in order, sharing that session.
4. Close the session.
5. Re-read `TODO.md`. If the first unchecked item changed, advance. If not, count a stall and retry.

The runner stops when there are no unchecked items left, when `max_iterations` is reached, or when the same item stalls `max_stalled_iterations` times in a row.

The default workflow scaffolded by `ghost-claude init`:

1. Execute the next TODO item.
2. Check that the new code has decent test coverage.
3. Run the relevant tests and fix failures.
4. Ask Codex for a review.
5. Mark the TODO item done.
6. Commit the changes with a descriptive message.

## Subcommands

```
ghost-claude run  [-config PATH] [-workspace DIR] [-dry-run]
ghost-claude init [-config PATH] [-workspace DIR] [-force]
ghost-claude help
```

If you omit the subcommand, `ghost-claude` behaves like `run`.

When `-workspace` is set, the default config path becomes `<workspace>/ghost-claude.yaml` and every relative path in the config resolves against that workspace.

## Config

The runner reads `ghost-claude.yaml` by default. Minimal example:

```yaml
workspace: .
todo_file: TODO.md
max_iterations: 0          # 0 means "run until TODO is empty"
max_stalled_iterations: 2  # how many no-progress iterations before aborting

claude:
  command: claude
  transport: tui           # "tui" drives the real UI; "print" uses claude --print
  startup_timeout: 30s
  session_strategy: session_id
  args:
    - --permission-mode
    - bypassPermissions

steps:
  - name: execute-next-todo
    type: claude
    prompt: |
      Execute the next incomplete TODO item from {{ .TodoFile }}.
      The current item is:
      {{ .NextTodo.Raw }}

  - name: run-tests
    type: claude
    prompt: |
      Run the relevant automated tests and fix any failures.

  - name: mark-todo-done
    type: claude
    prompt: |
      If this TODO item is fully complete, edit {{ .TodoFile }} and
      change this exact line from unchecked to checked:
      {{ .NextTodo.Raw }}
```

### Top-level fields

| Field                    | Default       | Meaning                                                            |
| ------------------------ | ------------- | ------------------------------------------------------------------ |
| `workspace`              | `.`           | Directory Claude runs in. Relative paths resolve from the config.  |
| `todo_file`              | `TODO.md`     | The checklist file the runner walks.                               |
| `max_iterations`         | `0`           | Hard cap on iterations. `0` means unlimited.                       |
| `max_stalled_iterations` | `2`           | Abort after this many no-progress iterations on the same item.     |
| `dry_run`                | `false`       | Render prompts and exec commands without running anything.         |

### `claude` block

| Field              | Default      | Meaning                                                                 |
| ------------------ | ------------ | ----------------------------------------------------------------------- |
| `command`          | `claude`     | Executable to launch.                                                   |
| `args`             | `[]`         | Extra CLI flags passed to Claude.                                       |
| `transport`        | `tui`        | `tui` drives the fullscreen UI inside a PTY. `print` uses `--print`.    |
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
| `timeout`           | all         | Go duration (e.g. `10m`). No timeout by default.                  |
| `continue_on_error` | all         | Log the failure and keep going instead of aborting.               |
| `disabled`          | all         | Skip the step.                                                    |

### Template data

Prompts, `command`, `working_dir`, and `env` values are rendered with Go's `text/template`. Available fields:

- `{{ .Workspace }}`
- `{{ .TodoFile }}`
- `{{ .ConfigPath }}`
- `{{ .Iteration }}`
- `{{ .SessionID }}`
- `{{ .NextTodo.Line }}` — 1-indexed line number in `TODO.md`
- `{{ .NextTodo.Raw }}` — the entire line, including the checkbox
- `{{ .NextTodo.Text }}` — just the task description
- `{{ .Now }}` — current time

## Notes & gotchas

- The runner only advances when the first incomplete checkbox in the TODO file changes. One of your steps has to edit the file.
- In TUI mode, YAML multiline prompts are flattened into one submitted message, because real newlines would be interpreted as separate messages by Claude's composer.
- In a fresh workspace, the runner auto-confirms Claude's trust dialog so the loop can start unattended.
- TUI automation detects "Claude is idle" from terminal-title transitions. If a future Claude release changes that behavior, the detector may need updating.
- `type: exec` lets you move deterministic steps (linters, formatters, arbitrary shell) out of Claude when that becomes cleaner.
