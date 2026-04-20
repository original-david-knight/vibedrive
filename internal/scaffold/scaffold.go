package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
)

const sampleConfig = `workspace: .
plan_file: ghost-plan.yaml
max_iterations: 0
max_stalled_iterations: 2
default_workflow: implement

claude:
  command: claude
  transport: tui
  startup_timeout: 30s
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
          Execute task {{ .Task.ID }} from {{ .PlanFile }}.
          Title: {{ .Task.Title }}
          
          Hard constraints to preserve:
          {{- range .Plan.Project.ConstraintFiles }}
          - {{ . }}
          {{- end }}
          {{- if .Task.Details }}

          Details:
          {{ .Task.Details }}
          {{- end }}
          {{- if .Task.ContextFiles }}

          Relevant files:
          {{- range .Task.ContextFiles }}
          - {{ . }}
          {{- end }}
          {{- end }}
          {{- if .Task.Acceptance }}

          Acceptance criteria:
          {{- range .Task.Acceptance }}
          - {{ . }}
          {{- end }}
          {{- end }}

          Make the necessary code changes in {{ .Workspace }}.
          Consult additional repository files only when they are needed to complete this task correctly.
          Do not edit {{ .PlanFile }} directly.

          Before you stop, write {{ .TaskResultPath }} as JSON with this schema:
          {"status":"done|in_progress|blocked","notes":"brief summary"}

          Set status to done only when the implementation work for this task is complete and ready for the configured automated verification commands.
          Set status to in_progress when meaningful progress was made but more work is still required.
          Set status to blocked when an external dependency, human decision, or missing prerequisite prevents completion.
          Keep notes short and specific.

      - name: codex-review
        type: claude
        fresh_session: true
        prompt: |
          If the current diff represents a non-trivial change, use /codex for a code review and address any actionable feedback.
          If your review changes the task's completion state or remaining work, update {{ .TaskResultPath }} to keep the status and notes accurate.
          Skip this step for trivial changes and leave {{ .TaskResultPath }} unchanged.

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
          - --message
          - "{{- if .Task.CommitMessage -}}{{ .Task.CommitMessage }}{{- else -}}{{ .Task.Title }}{{- end -}}"

  checkpoint:
    steps:
      - name: run-checkpoint
        type: claude
        fresh_session: true
        prompt: |
          Execute the checkpoint task {{ .Task.ID }} from {{ .PlanFile }}.
          Title: {{ .Task.Title }}

          Hard constraints to preserve:
          {{- range .Plan.Project.ConstraintFiles }}
          - {{ . }}
          {{- end }}
          {{- if .Task.Details }}

          Details:
          {{ .Task.Details }}
          {{- end }}
          {{- if .Task.Acceptance }}

          Acceptance criteria:
          {{- range .Task.Acceptance }}
          - {{ . }}
          {{- end }}
          {{- end }}

          Run the full required verification for this checkpoint, fix any regressions you find, and leave the repository green before moving on.
          Consult additional repository files only when they are needed to complete this checkpoint correctly.
          Do not edit {{ .PlanFile }} directly.

          Before you stop, write {{ .TaskResultPath }} as JSON with this schema:
          {"status":"done|in_progress|blocked","notes":"brief summary"}

          Set status to done only when the checkpoint work is complete and ready for the configured automated verification commands.
          Set status to in_progress when meaningful progress was made but more work is still required.
          Set status to blocked when an external dependency, human decision, or missing prerequisite prevents completion.
          Keep notes short and specific.

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
          - --message
          - "{{- if .Task.CommitMessage -}}{{ .Task.CommitMessage }}{{- else -}}{{ .Task.Title }}{{- end -}}"
`

func Write(configPath string, force bool) error {
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

	return nil
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o644)
}
