# ghost-claude

`ghost-claude` is a terminal-native Go runner that loops over a `TODO.md` file and drives Claude Code through a configurable workflow.

The default transport launches Claude's real fullscreen TUI inside a PTY, sends prompts as terminal input, and waits for Claude to return to an idle prompt before sending the next step.
Each TODO item gets a fresh Claude session, so context does not carry from one TODO to the next.

## Quick Start

```bash
go run ./cmd/ghost-claude init
go run ./cmd/ghost-claude run
```

To target another repo without changing directories:

```bash
go run ./cmd/ghost-claude init --workspace /path/to/repo
go run ./cmd/ghost-claude run --workspace /path/to/repo
```

The generated workflow matches your current loop:

1. Execute the next TODO item.
2. Check that the new code has decent test coverage.
3. Run the relevant tests and fix failures.
4. Ask Codex for a review.
5. Mark the TODO item done.

## Config

The runner reads `ghost-claude.yaml` by default.

If `--workspace /path/to/repo` is provided, the default config path becomes `/path/to/repo/ghost-claude.yaml`.

Example:

```yaml
workspace: .
todo_file: TODO.md
max_iterations: 0
max_stalled_iterations: 2

claude:
  command: claude
  transport: tui
  startup_timeout: 30s
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

  - name: check-coverage
    type: claude
    prompt: |
      Check whether the new or changed code you just wrote has decent test coverage.
      Run coverage-related test commands as needed.
      If tests are missing or weak for the behavior you changed, add or improve them before moving on.

  - name: run-tests
    type: claude
    prompt: |
      Run the relevant automated tests for the code you just wrote or changed.
      Fix any failures you find, then rerun the relevant tests until they pass.
```

Template data available to prompts and exec commands:

- `{{ .Workspace }}`
- `{{ .TodoFile }}`
- `{{ .Iteration }}`
- `{{ .SessionID }}`
- `{{ .NextTodo.Line }}`
- `{{ .NextTodo.Raw }}`
- `{{ .NextTodo.Text }}`

## Notes

- `max_iterations: 0` means "run until there are no unchecked TODO items left".
- The runner only advances when the first incomplete checkbox in the TODO file changes.
- `max_stalled_iterations` controls how many times the same first unchecked item can repeat before the runner aborts. The default is `2`, which gives Claude one automatic retry before stopping.
- Each TODO iteration gets a new Claude session; the steps inside that one iteration share a session.
- `type: exec` exists so you can move some steps out of Claude later if that becomes cleaner.
- `--workspace /path/to/repo` lets you run the tool against another workspace without changing your current directory.
- `claude.transport: tui` drives the fullscreen UI. Set `claude.transport: print` if you want the old `claude --print` behavior instead.
- In TUI mode, YAML multiline prompts are flattened into one submitted message because literal line breaks are interpreted by Claude's composer as separate messages.
- In a fresh workspace, the runner auto-confirms Claude's trust dialog so the loop can start.
- TUI automation currently detects readiness from Claude's terminal title transitions. If a future Claude release changes that behavior, the detector may need adjustment.
