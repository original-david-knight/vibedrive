package claude

import "testing"

func TestTitleParserConsume(t *testing.T) {
	parser := &titleParser{}
	chunks := [][]byte{
		[]byte("\x1b]0;✳ Claude"),
		[]byte(" Code\x07ignored"),
		[]byte("\x1b]0;⠂ Claude Code\x07"),
	}

	var titles []string
	for _, chunk := range chunks {
		titles = append(titles, parser.consume(chunk)...)
	}

	if len(titles) != 2 {
		t.Fatalf("expected 2 titles, got %d", len(titles))
	}
	if titles[0] != "✳ Claude Code" {
		t.Fatalf("unexpected first title %q", titles[0])
	}
	if titles[1] != "⠂ Claude Code" {
		t.Fatalf("unexpected second title %q", titles[1])
	}
}

func TestClassifyTitle(t *testing.T) {
	tests := []struct {
		title string
		want  string
		ok    bool
	}{
		{title: "✳ Claude Code", want: "idle", ok: true},
		{title: "⠂ Claude Code", want: "busy", ok: true},
		{title: "✳ Implement THREAT_MODEL.md code changes", want: "idle", ok: true},
		{title: "⠂ Implement THREAT_MODEL.md code changes", want: "busy", ok: true},
		{title: "", want: "", ok: false},
	}

	for _, test := range tests {
		got, ok := classifyTitle(test.title)
		if ok != test.ok {
			t.Fatalf("title %q: expected ok=%v, got %v", test.title, test.ok, ok)
		}
		if got != test.want {
			t.Fatalf("title %q: expected %q, got %q", test.title, test.want, got)
		}
	}
}

func TestNormalizePromptForTUI(t *testing.T) {
	got := normalizePromptForTUI("first line\n\nsecond line\r\n  third")
	want := "first line second line third"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
