package render

import "testing"

func TestString(t *testing.T) {
	out, err := String("hello {{ .Name }}", map[string]string{"Name": "world"})
	if err != nil {
		t.Fatalf("String returned error: %v", err)
	}
	if out != "hello world" {
		t.Fatalf("unexpected output %q", out)
	}
}

func TestStringFailsOnMissingKey(t *testing.T) {
	if _, err := String("hello {{ .Missing }}", map[string]string{"Name": "world"}); err == nil {
		t.Fatal("expected missing key error")
	}
}
