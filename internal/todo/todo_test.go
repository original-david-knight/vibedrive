package todo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindNextIncomplete(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TODO.md")

	content := `# TODO

- [x] done
- [ ] first
- [ ] second
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	item, err := FindNextIncomplete(path)
	if err != nil {
		t.Fatalf("FindNextIncomplete returned error: %v", err)
	}

	if item.Line != 4 {
		t.Fatalf("expected line 4, got %d", item.Line)
	}
	if item.Text != "first" {
		t.Fatalf("expected text %q, got %q", "first", item.Text)
	}
}

func TestFindNextIncompleteReturnsErrorWhenDone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "TODO.md")

	content := `- [x] done
- [X] also done
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	if _, err := FindNextIncomplete(path); err != ErrNoIncompleteItems {
		t.Fatalf("expected ErrNoIncompleteItems, got %v", err)
	}
}
