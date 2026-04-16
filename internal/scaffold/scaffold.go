package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
)

const sampleConfig = `workspace: .
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

      Make the necessary code changes in {{ .Workspace }}.

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

  - name: codex-review
    type: claude
    prompt: |
      Ask Codex for a code review of the current diff and address any actionable feedback.
      Use the local codex CLI if needed.

  - name: mark-todo-done
    type: claude
    prompt: |
      Edit {{ .TodoFile }} directly.
      If this TODO item is fully complete, change this exact line from unchecked to checked:
      {{ .NextTodo.Raw }}

      ghost-claude only proceeds to the next item when the first incomplete checkbox changes.

      If it is not fully complete, leave it unchecked and explain why.

  - name: commit-changes
    type: claude
    prompt: |
      Stage and commit all changes from this iteration with git.
      Write a clear, conventional commit message that summarizes what this TODO item accomplished:
      {{ .NextTodo.Raw }}

      Include both the code changes and the {{ .TodoFile }} update in a single commit.
      If there is nothing to commit, skip this step.
`

const sampleTODO = `# TODO

- [ ] Replace this item with a real task.
- [ ] Add another task.
`

func Write(configPath string, force bool) error {
	todoPath := filepath.Join(filepath.Dir(configPath), "TODO.md")

	if !force {
		if _, err := os.Stat(configPath); err == nil {
			return fmt.Errorf("%s already exists; use -force to overwrite", configPath)
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	if err := writeFile(configPath, []byte(sampleConfig)); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", configPath)

	if !force {
		if _, err := os.Stat(todoPath); err == nil {
			fmt.Printf("Skipped %s (already exists)\n", todoPath)
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	if err := writeFile(todoPath, []byte(sampleTODO)); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", todoPath)
	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
