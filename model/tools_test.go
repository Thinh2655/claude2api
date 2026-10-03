package model

import "testing"

func TestExtractToolCalls(t *testing.T) {
	text := "```toolcall {\"name\":\"todo_write\",\"arguments\":{\"todos\":[{\"content\":\"x\"}]}}```"
	cleaned, calls := ExtractToolCalls(text)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "todo_write" {
		t.Fatalf("wrong name: %s", calls[0].Function.Name)
	}
	if calls[0].Type != "function" || calls[0].ID == "" {
		t.Fatalf("bad envelope: %+v", calls[0])
	}
	if cleaned != "" {
		t.Fatalf("expected empty cleaned text, got %q", cleaned)
	}

	cleaned, calls = ExtractToolCalls("just text")
	if len(calls) != 0 || cleaned != "just text" {
		t.Fatalf("plain text must pass through: %q %v", cleaned, calls)
	}
}
