package core

import (
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"claude2api/config"

	"github.com/gin-gonic/gin"
)

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
	if got := resolveUpstreamTools([]map[string]interface{}{{"name": "nope"}}, defs); len(got) != 0 {
		t.Fatal("all-unknown input must strip defaults (custom tools go via prompt)")
	}
	if got := resolveUpstreamTools([]map[string]interface{}{{"name": "web_search"}}, defs); len(got) != 1 {
		t.Fatal("explicit web_search must be kept")
	}
}

func TestDisableToolsKeepsOnlyMCP(t *testing.T) {
	mcpEnabled := config.ConfigInstance.EnableMCP
	config.ConfigInstance.EnableMCP = true
	org := t.Name()
	mcpCache[org] = mcpCacheEntry{
		tools:     []map[string]interface{}{{"name": "lookup", "mcp_server_uuid": "remote"}},
		expiresAt: time.Now().Add(time.Minute),
	}
	t.Cleanup(func() {
		config.ConfigInstance.EnableMCP = mcpEnabled
		delete(mcpCache, org)
	})

	client := NewClient("test-session", "", "")
	client.SetOrgID(org)
	builtin := map[string]bool{"web_search": false, "artifacts": false, "repl": false}
	client.SetTools(nil, false, builtin)
	assertJSONEqual(t, client.defaultAttrs["tools"], `[]`)
	client.AttachMCPTools()
	assertJSONEqual(t, client.defaultAttrs["tools"], `[{"name":"lookup","mcp_server_uuid":"remote"}]`)

	const toolCall = "```toolcall {\"name\":\"client_function\"}```"
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
			event := fmt.Sprintf(`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":%q}}`+"\n\n", toolCall)
			if err := client.HandleResponse(io.NopCloser(strings.NewReader(event)), stream, ctx); err != nil {
				t.Fatal(err)
			}
			if body := recorder.Body.String(); strings.Contains(body, `"tool_calls"`) || !strings.Contains(body, "```toolcall") {
				t.Fatalf("expected toolcall fence as text, got %s", body)
			}
		})
	}
}

func TestBuiltinToolsCanBeToggledIndependently(t *testing.T) {
	builtin := map[string]bool{"web_search": true, "artifacts": false, "repl": false}
	client := NewClient("test-session", "", "")
	client.SetTools(nil, false, builtin)
	assertJSONEqual(t, client.defaultAttrs["tools"], `[{"type":"web_search_v0","name":"web_search"}]`)

	client = NewClient("test-session", "", "")
	client.SetTools([]map[string]interface{}{{"name": "websearch"}, {"name": "artifact"}, {"name": "code_interpreter"}}, true, builtin)
	assertJSONEqual(t, client.defaultAttrs["tools"], `[{"type":"web_search_v0","name":"web_search"}]`)
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

func TestIsSilentCodeTool(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"write_file", true},
		{"mcp__tool__write_file", true},
		{"tool:write_file", true},
		{"tool:mcp__write_file", true},
		{"WRITE_FILE", true},
		{"  write_file  ", true},
		{"\twrite_file\n", true},

		{"read_file", false},
		{"mcp__tool__read_file", false},
		{"artifacts", false},
		{"create_file", false},
		{"", false},
		{"mcp__tool__", false},
		{"tool:", false},
		{"write_file__", false},
		{"write_file:", false},
		{"mcp__tool__write_file:", false},
		{"my_write_file", false},
		{"write_file_v2", false},
	}
	for _, c := range cases {
		if got := isSilentCodeTool(c.in); got != c.want {
			t.Fatalf("isSilentCodeTool(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
