package core

import "testing"

func TestUpstreamTool(t *testing.T) {
	cases := []struct {
		in   map[string]interface{}
		want string // "" = dropped
	}{
		{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "web_search"}}, "web_search_v0"},
		{map[string]interface{}{"name": "artifacts"}, "artifacts_v0"},
		{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "code_interpreter"}}, "repl_v0"},
		{map[string]interface{}{"type": "function", "function": map[string]interface{}{"name": "my_custom_fn"}}, ""},
	}
	for _, c := range cases {
		got := upstreamTool(c.in)
		if c.want == "" && got != nil {
			t.Fatalf("expected drop for %v, got %v", c.in, got)
		}
		if c.want != "" && (got == nil || got["type"] != c.want) {
			t.Fatalf("expected %s for %v, got %v", c.want, c.in, got)
		}
	}

	defs := []map[string]interface{}{{"type": "web_search_v0", "name": "web_search"}}
	if got := resolveUpstreamTools(nil, defs); len(got) != 1 {
		t.Fatal("empty input must keep defaults")
	}
	if got := resolveUpstreamTools([]map[string]interface{}{{"name": "nope"}}, defs); len(got) != 1 {
		t.Fatal("all-unknown input must keep defaults")
	}
}

func TestDetectToolLanguage(t *testing.T) {
	cases := []struct {
		input toolUseInput
		want  string
	}{
		{toolUseInput{Language: "python"}, "python"},
		{toolUseInput{Type: "text/html"}, "html"},
		{toolUseInput{Path: "main.go"}, "go"},
		{toolUseInput{Path: "/app/src/index.tsx"}, "typescript"},
		{toolUseInput{Path: "script.py"}, "python"},
		{toolUseInput{Path: "unknown.xyz"}, "xyz"},
		{toolUseInput{}, "text"},
	}
	for _, c := range cases {
		if got := detectToolLanguage(c.input); got != c.want {
			t.Fatalf("detectToolLanguage(%+v) = %s, want %s", c.input, got, c.want)
		}
	}
}

func TestFormatToolThinkingLog(t *testing.T) {
	cases := []struct {
		input toolUseInput
		want  string
	}{
		{toolUseInput{Description: "Writing animation script", Command: "create", Path: "anim.py"}, "Writing animation script | create anim.py"},
		{toolUseInput{Command: "update", Path: "main.go"}, "update main.go"},
		{toolUseInput{Query: "claude api"}, "Search: claude api"},
		{toolUseInput{}, ""},
	}
	for _, c := range cases {
		if got := formatToolThinkingLog(c.input); got != c.want {
			t.Fatalf("formatToolThinkingLog(%+v) = %s, want %s", c.input, got, c.want)
		}
	}
}
