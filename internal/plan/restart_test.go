package plan

import "testing"

func TestResetProgressResetsStatusesAndClearsNotes(t *testing.T) {
	file := &File{
		Tasks: []Task{
			{ID: "done-task", Title: "Done task", Status: StatusDone, Notes: "finished cleanly"},
			{ID: "blocked-task", Title: "Blocked task", Status: StatusBlocked, Notes: "missing seed data"},
			{ID: "manual-task", Title: "Manual task", Status: StatusManual, Notes: "needs human review"},
			{ID: "in-progress-task", Title: "In progress task", Status: StatusInProgress, Notes: "split the migration first"},
		},
	}

	file.ResetProgress()

	for _, task := range file.Tasks {
		if task.Status != StatusTodo {
			t.Fatalf("expected task %q status %q, got %q", task.ID, StatusTodo, task.Status)
		}
		if task.Notes != "" {
			t.Fatalf("expected task %q notes to be cleared, got %q", task.ID, task.Notes)
		}
	}
}
