package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func assertJSONEqual(t *testing.T, got any, want string) {
	t.Helper()
	gotBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal got: %v", err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(gotBytes, &gotValue); err != nil {
		t.Fatalf("unmarshal got: %v", err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatalf("unmarshal want: %v", err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("json mismatch\ngot:  %s\nwant: %s", gotBytes, want)
	}
}

func strPtr(s string) *string {
	return &s
}

func TestBuildMCPInstallDefault(t *testing.T) {
	install, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
		Headers: []MCPHeader{{}},
	})
	if err != nil {
		t.Fatalf("BuildMCPInstall: %v", err)
	}
	if install.Inputs == nil {
		t.Fatal("Inputs must be non-nil so it marshals as []")
	}
	assertJSONEqual(t, install, `{
		"type": "custom",
		"transport": "streamable-http",
		"posture": "no_auth",
		"inputs": []
	}`)
}

func TestBuildMCPInstallStaticHeaders(t *testing.T) {
	install, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
		AuthMode: " no_auth ",
		Headers: []MCPHeader{
			{Name: " X-Api-Key ", Value: " secret ", Required: true},
			{Name: "X-Optional", Value: "  "},
		},
	})
	if err != nil {
		t.Fatalf("BuildMCPInstall: %v", err)
	}
	assertJSONEqual(t, install, `{
		"type": "custom",
		"transport": "streamable-http",
		"posture": "auth_required",
		"auth_method": {
			"type": "static_headers",
			"mcp_request_headers": [
				{"name": "X-Api-Key", "scope": "organization", "required": true},
				{"name": "X-Optional", "scope": "organization", "required": false}
			]
		},
		"inputs": [
			{"type": "mcp_request_header", "name": "X-Api-Key", "value": "secret"}
		]
	}`)
}

func TestBuildMCPInstallAllowsAuthorizationWithoutOAuth(t *testing.T) {
	install, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
		AuthMode: "no_auth",
		Headers: []MCPHeader{
			{Name: "Authorization", Value: "Bearer token", Required: true},
		},
	})
	if err != nil {
		t.Fatalf("BuildMCPInstall: %v", err)
	}
	assertJSONEqual(t, install, `{
		"type": "custom",
		"transport": "streamable-http",
		"posture": "auth_required",
		"auth_method": {
			"type": "static_headers",
			"mcp_request_headers": [
				{"name": "Authorization", "scope": "organization", "required": true}
			]
		},
		"inputs": [
			{"type": "mcp_request_header", "name": "Authorization", "value": "Bearer token"}
		]
	}`)
}

func TestBuildMCPInstallAllowsUnderscoreAndHyphenNames(t *testing.T) {
	install, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
		Headers: []MCPHeader{
			{Name: "X_Key", Value: "one"},
			{Name: "X-Key", Value: "two"},
		},
	})
	if err != nil {
		t.Fatalf("BuildMCPInstall: %v", err)
	}
	if len(install.Inputs) != 2 {
		t.Fatalf("inputs = %d, want 2", len(install.Inputs))
	}
}

func TestBuildMCPInstallOAuthModes(t *testing.T) {
	tests := []struct {
		name string
		url  string
		opts MCPInstallOptions
		want string
	}{
		{
			name: "cimd auth required",
			url:  "https://example.com/mcp",
			opts: MCPInstallOptions{
				AuthMode:     "auth_required",
				Registration: " cimd ",
			},
			want: `{
				"type": "custom",
				"transport": "streamable-http",
				"posture": "auth_required",
				"auth_method": {
					"type": "oauth",
					"registration": {"type": "cimd"},
					"mcp_request_headers": []
				},
				"inputs": []
			}`,
		},
		{
			name: "dcr defer auth sse",
			url:  "https://example.com/sse/?x=1",
			opts: MCPInstallOptions{
				AuthMode:     "defer_auth",
				Registration: "dcr",
			},
			want: `{
				"type": "custom",
				"transport": "sse",
				"posture": "defer_auth",
				"auth_method": {
					"type": "oauth",
					"registration": {"type": "dcr"},
					"mcp_request_headers": []
				},
				"inputs": []
			}`,
		},
		{
			name: "static with secret and optional header",
			url:  "https://example.com/mcp",
			opts: MCPInstallOptions{
				AuthMode:     "auth_required",
				Registration: "static_per_installation",
				ClientID:     " client-id ",
				ClientSecret: " client-secret ",
				Headers: []MCPHeader{
					{Name: " X-Api-Key ", Value: " header-value "},
				},
			},
			want: `{
				"type": "custom",
				"transport": "streamable-http",
				"posture": "auth_required",
				"auth_method": {
					"type": "oauth",
					"registration": {
						"type": "static_per_installation",
						"client_credential": {"type": "client_secret"}
					},
					"mcp_request_headers": [
						{"name": "X-Api-Key", "scope": "organization", "required": false}
					]
				},
				"inputs": [
					{"type": "oauth_method_input", "name": "client_id", "value": "client-id"},
					{"type": "oauth_client_credential", "name": "client_secret", "value": "client-secret"},
					{"type": "mcp_request_header", "name": "X-Api-Key", "value": "header-value"}
				]
			}`,
		},
		{
			name: "static without secret",
			url:  "https://example.com/mcp",
			opts: MCPInstallOptions{
				AuthMode:     "auth_required",
				Registration: "static_per_installation",
				ClientID:     "client-id",
				ClientSecret: "   ",
			},
			want: `{
				"type": "custom",
				"transport": "streamable-http",
				"posture": "auth_required",
				"auth_method": {
					"type": "oauth",
					"registration": {
						"type": "static_per_installation",
						"client_credential": {"type": "none"}
					},
					"mcp_request_headers": []
				},
				"inputs": [
					{"type": "oauth_method_input", "name": "client_id", "value": "client-id"}
				]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			install, err := BuildMCPInstall(tt.url, tt.opts)
			if err != nil {
				t.Fatalf("BuildMCPInstall: %v", err)
			}
			assertJSONEqual(t, install, tt.want)
		})
	}
}

func TestBuildMCPInstallValidation(t *testing.T) {
	fourHeaders := []MCPHeader{
		{Name: "H1", Value: "v1"},
		{Name: "H2", Value: "v2"},
		{Name: "H3", Value: "v3"},
		{Name: "H4", Value: "v4"},
	}
	tests := []struct {
		name    string
		url     string
		opts    MCPInstallOptions
		wantErr string
	}{
		{
			name:    "invalid auth mode",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{AuthMode: "oauth"},
			wantErr: "authMode không hợp lệ",
		},
		{
			name:    "oauth missing registration",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{AuthMode: "auth_required", Registration: " "},
			wantErr: "registration là bắt buộc",
		},
		{
			name:    "oauth invalid registration",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{AuthMode: "auth_required", Registration: "manual"},
			wantErr: "registration không hợp lệ",
		},
		{
			name:    "cimd rejects client credentials",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{AuthMode: "auth_required", Registration: "cimd", ClientID: "id"},
			wantErr: "chỉ dùng với registration=static_per_installation",
		},
		{
			name:    "static missing client id",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{AuthMode: "auth_required", Registration: "static_per_installation", ClientSecret: "secret"},
			wantErr: "clientId là bắt buộc",
		},
		{
			name:    "no auth rejects registration",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Registration: "cimd"},
			wantErr: "chỉ dùng khi bật OAuth",
		},
		{
			name:    "no auth rejects client id",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{ClientID: "id"},
			wantErr: "chỉ dùng khi bật OAuth",
		},
		{
			name:    "header name without value",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: []MCPHeader{{Name: " ", Value: "v"}}},
			wantErr: "tên request header không được để trống",
		},
		{
			name:    "header name with space",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: []MCPHeader{{Name: "Bad Name", Value: "v"}}},
			wantErr: "tên request header không hợp lệ",
		},
		{
			name:    "header name leading dot",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: []MCPHeader{{Name: ".abc", Value: "v"}}},
			wantErr: "tên request header không hợp lệ",
		},
		{
			name:    "header name trailing dot",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: []MCPHeader{{Name: "abc.", Value: "v"}}},
			wantErr: "tên request header không hợp lệ",
		},
		{
			name:    "header name double dot",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: []MCPHeader{{Name: "a..b", Value: "v"}}},
			wantErr: "tên request header không hợp lệ",
		},
		{
			name:    "header name dot before hyphen",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: []MCPHeader{{Name: "a.-b", Value: "v"}}},
			wantErr: "tên request header không hợp lệ",
		},
		{
			name:    "duplicate header",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: []MCPHeader{{Name: "X-Key", Value: "1"}, {Name: "x-key", Value: "2"}}},
			wantErr: "bị trùng",
		},
		{
			name:    "alike header",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: []MCPHeader{{Name: "X.Api-Key", Value: "1"}, {Name: "x_api_key", Value: "2"}}},
			wantErr: "bị trùng",
		},
		{
			name:    "too many headers",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: append(fourHeaders, MCPHeader{Name: "H5", Value: "v5"})},
			wantErr: "tối đa 4",
		},
		{
			name: "required header missing value",
			url:  "https://example.com/mcp",
			opts: MCPInstallOptions{Headers: []MCPHeader{
				{Name: "X-Required", Required: true},
				{Name: "X-Valid", Value: "v"},
			}},
			wantErr: "bắt buộc phải có value",
		},
		{
			name: "no auth all optional headers empty",
			url:  "https://example.com/mcp",
			opts: MCPInstallOptions{Headers: []MCPHeader{
				{Name: "X-One"},
				{Name: "X-Two", Value: " "},
			}},
			wantErr: "cần ít nhất một request header có value",
		},
		{
			name:    "oauth rejects authorization header",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{AuthMode: "auth_required", Registration: "cimd", Headers: []MCPHeader{{Name: "AUTHORIZATION", Value: "Bearer x"}}},
			wantErr: "Authorization do OAuth quản lý",
		},
		{
			name:    "control character in header value",
			url:     "https://example.com/mcp",
			opts:    MCPInstallOptions{Headers: []MCPHeader{{Name: "X-Key", Value: "ab\r\ncd"}}},
			wantErr: "giá trị request header không hợp lệ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			install, err := BuildMCPInstall(tt.url, tt.opts)
			if err == nil {
				t.Fatalf("expected error, got install %#v", install)
			}
			if install != nil {
				t.Fatalf("expected nil install, got %#v", install)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestBuildMCPInstallLimits(t *testing.T) {
	name64 := strings.Repeat("a", 64)
	name65 := strings.Repeat("a", 65)
	value4096 := strings.Repeat("x", 4096)
	value4097 := strings.Repeat("x", 4097)

	t.Run("name 64 valid", func(t *testing.T) {
		if _, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
			Headers: []MCPHeader{{Name: name64, Value: "v"}},
		}); err != nil {
			t.Fatalf("BuildMCPInstall: %v", err)
		}
	})
	t.Run("name 65 invalid", func(t *testing.T) {
		if _, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
			Headers: []MCPHeader{{Name: name65, Value: "v"}},
		}); err == nil {
			t.Fatal("expected error for 65-char header name")
		}
	})
	t.Run("value 4096 valid", func(t *testing.T) {
		install, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
			Headers: []MCPHeader{{Name: "X-Key", Value: value4096}},
		})
		if err != nil {
			t.Fatalf("BuildMCPInstall: %v", err)
		}
		if got := len(install.Inputs[0].Value); got != 4096 {
			t.Fatalf("value length = %d, want 4096", got)
		}
	})
	t.Run("value 4097 invalid", func(t *testing.T) {
		if _, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
			Headers: []MCPHeader{{Name: "X-Key", Value: value4097}},
		}); err == nil {
			t.Fatal("expected error for 4097-byte header value")
		}
	})
	t.Run("exactly four headers valid", func(t *testing.T) {
		if _, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
			Headers: []MCPHeader{
				{Name: "H1", Value: "v1"},
				{Name: "H2", Value: "v2"},
				{Name: "H3", Value: "v3"},
				{Name: "H4", Value: "v4"},
			},
		}); err != nil {
			t.Fatalf("BuildMCPInstall: %v", err)
		}
	})
}

func TestBuildMCPInstallErrorDoesNotLeakSecrets(t *testing.T) {
	_, err := BuildMCPInstall("https://example.com/mcp", MCPInstallOptions{
		AuthMode:     "auth_required",
		Registration: "static_per_installation",
		ClientID:     "client-id",
		ClientSecret: "super-secret",
		Headers:      []MCPHeader{{Name: "Bad Name", Value: "header-secret"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "super-secret") || strings.Contains(err.Error(), "header-secret") {
		t.Fatalf("error leaks secret: %q", err.Error())
	}
}

func TestMCPTransportForURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://example.com/sse", "sse"},
		{"https://example.com/sse/", "sse"},
		{"https://example.com/sse//", "sse"},
		{"https://example.com/sse?x=1", "sse"},
		{"https://example.com/sse#fragment", "sse"},
		{"https://example.com/api/sse", "sse"},
		{" https://example.com/sse/ ", "sse"},
		{"https://example.com/messages", "streamable-http"},
		{"https://example.com/sse/messages", "streamable-http"},
		{"https://example.com/SSE", "streamable-http"},
		{"https://example.com/sse2", "streamable-http"},
		{"https://sse.example.com", "streamable-http"},
		{"", "streamable-http"},
		{"://bad", "streamable-http"},
	}

	for _, tt := range tests {
		if got := mcpTransportForURL(tt.url); got != tt.want {
			t.Errorf("mcpTransportForURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestNormalizeMCPURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"HTTPS://Example.COM/MCP/", "https://example.com/MCP"},
		{"https://example.com/sse///", "https://example.com/sse"},
		{"https://example.com/mcp?x=1", "https://example.com/mcp?x=1"},
		{"https://example.com/", "https://example.com"},
		{" https://Example.com:443/MCP// ", "https://example.com:443/MCP"},
		{"://bad/", "://bad"},
	}

	for _, tt := range tests {
		if got := normalizeMCPURL(tt.url); got != tt.want {
			t.Errorf("normalizeMCPURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestMCPServerMatchesDefaultInstall(t *testing.T) {
	install := &MCPInstall{Transport: "streamable-http"}

	tests := []struct {
		name   string
		server remoteMCPServer
		want   bool
	}{
		{
			name:   "nil summary is legacy",
			server: remoteMCPServer{},
			want:   true,
		},
		{
			name: "matching no auth",
			server: remoteMCPServer{InstallSummary: &remoteMCPInstallSummary{
				Transport: "streamable-http",
				Posture:   "no_auth",
			}},
			want: true,
		},
		{
			name: "empty selected method",
			server: remoteMCPServer{InstallSummary: &remoteMCPInstallSummary{
				Transport:          "streamable-http",
				Posture:            "no_auth",
				SelectedMethodType: strPtr(""),
			}},
			want: true,
		},
		{
			name: "transport mismatch",
			server: remoteMCPServer{InstallSummary: &remoteMCPInstallSummary{
				Transport: "sse",
				Posture:   "no_auth",
			}},
			want: false,
		},
		{
			name: "auth required",
			server: remoteMCPServer{InstallSummary: &remoteMCPInstallSummary{
				Transport: "streamable-http",
				Posture:   "auth_required",
			}},
			want: false,
		},
		{
			name: "selected oauth method",
			server: remoteMCPServer{InstallSummary: &remoteMCPInstallSummary{
				Transport:          "streamable-http",
				Posture:            "no_auth",
				SelectedMethodType: strPtr("oauth"),
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mcpServerMatchesDefaultInstall(tt.server, install); got != tt.want {
				t.Fatalf("mcpServerMatchesDefaultInstall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseUpstreamError(t *testing.T) {
	message, uuid := parseUpstreamError([]byte(`{"error":{"message":"boom","details":{"existing_connector_uuid":"u-1"}}}`))
	if message != "boom" || uuid != "u-1" {
		t.Fatalf("parseUpstreamError() = %q, %q", message, uuid)
	}

	body := strings.Repeat("ế", 300)
	message, uuid = parseUpstreamError([]byte("  " + body + "  "))
	if uuid != "" {
		t.Fatalf("uuid = %q, want empty", uuid)
	}
	if !utf8.ValidString(message) {
		t.Fatalf("message is not valid UTF-8: %q", message)
	}
	if !strings.HasSuffix(message, "...") {
		t.Fatalf("message = %q, want suffix ...", message)
	}
	if got := len([]rune(message)); got != 203 {
		t.Fatalf("message rune length = %d, want 203", got)
	}
}
