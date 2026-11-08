package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"claude2api/config"
	"claude2api/logger"
)

// MCPToolPermissions says which permission mode each MCP tool gets when the
// account's enabled_mcp_tools setting is written. Tools overrides Default by
// tool name.
type MCPToolPermissions struct {
	Default string
	Tools   map[string]string
}

// ModeFor returns the effective mode of a tool. Anything unknown falls back to
// the default mode and then to always_allow.
func (p MCPToolPermissions) ModeFor(tool string) string {
	if m, ok := config.NormalizeMCPToolMode(p.Tools[tool]); ok {
		return m
	}
	if m, ok := config.NormalizeMCPToolMode(p.Default); ok {
		return m
	}
	return config.MCPModeAlwaysAllow
}

// mcpModeResult is the outcome of applyMCPToolModes.
type mcpModeResult struct {
	WantTrue    []string // keys that must read back true
	WantNotTrue []string // keys that must not read back true
	AlwaysAllow int      // tools set to always_allow
	UseTool     int      // tools set to use_tool
	MissingKey  int      // tools for which claude.ai gave no always_approved_key
}

// applyMCPToolModes writes the enabled_mcp_tools keys of one server's tools
// into enabled, following perms. claude.ai stores two keys per tool:
// "<uuid>:<tool>" (enabled) and an always-approved key with a hash suffix.
// always_allow sets both to true; use_tool sets only the first, so claude.ai
// asks for approval on each call. The settings PATCH merges maps, so a key is
// removed by setting it to nil rather than omitting it.
func applyMCPToolModes(enabled map[string]interface{}, serverUUID string, tools []remoteMCPTool, perms MCPToolPermissions) mcpModeResult {
	var res mcpModeResult
	for _, tool := range tools {
		if tool.Name == "" {
			continue
		}
		enabledKey := tool.EnabledKey
		if enabledKey == "" {
			enabledKey = fmt.Sprintf("%s:%s", serverUUID, tool.Name)
		}
		enabled[enabledKey] = true

		useTool := perms.ModeFor(tool.Name) == config.MCPModeUseTool
		if useTool {
			res.UseTool++
		} else {
			res.AlwaysAllow++
		}
		if tool.AlwaysApprovedKey == "" {
			res.MissingKey++
			continue
		}
		if useTool {
			enabled[tool.AlwaysApprovedKey] = nil
			res.WantNotTrue = append(res.WantNotTrue, tool.AlwaysApprovedKey)
		} else {
			enabled[tool.AlwaysApprovedKey] = true
			res.WantTrue = append(res.WantTrue, tool.AlwaysApprovedKey)
		}
	}
	return res
}

// mcpSettingMismatch returns the first key of saved that is not in the wanted
// state, or "" when everything matches.
func mcpSettingMismatch(saved map[string]interface{}, wantTrue, wantNotTrue []string) string {
	for _, key := range wantTrue {
		if v, ok := saved[key]; !ok || v != true {
			return key
		}
	}
	for _, key := range wantNotTrue {
		if v, ok := saved[key]; ok && v == true {
			return key
		}
	}
	return ""
}

// describeMCPModes summarizes how many tools got each mode, for result messages.
func describeMCPModes(always, ask int) string {
	switch {
	case always+ask == 0:
		return "chưa áp quyền"
	case ask == 0:
		return "Always Allow"
	case always == 0:
		return "Use tool"
	default:
		return fmt.Sprintf("%d Always Allow, %d Use tool", always, ask)
	}
}

// writeEnabledMCPTools stores enabled in the account's enabled_mcp_tools
// setting, then reads it back until wantTrue / wantNotTrue hold. written is
// true once the PATCH was accepted, so a non-nil error with written=true means
// only the verification failed.
func (c *Client) writeEnabledMCPTools(enabled map[string]interface{}, wantTrue, wantNotTrue []string) (written bool, err error) {
	if err := c.UpdateUserSetting("enabled_mcp_tools", enabled); err != nil {
		return false, fmt.Errorf("write setting: %w", err)
	}
	// PATCH returns 202 (accepted), so allow the read-back a moment to catch
	// up before reporting a failure.
	var verifyErr error
	mismatch := ""
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		saved, err := c.fetchEnabledMCPTools()
		if err != nil {
			verifyErr = fmt.Errorf("verify setting: %w", err)
			continue
		}
		verifyErr = nil
		mismatch = mcpSettingMismatch(saved, wantTrue, wantNotTrue)
		if mismatch == "" {
			break
		}
	}
	if verifyErr != nil {
		return true, verifyErr
	}
	if mismatch != "" {
		return true, fmt.Errorf("setting %q was not persisted", mismatch)
	}
	return true, nil
}

// fetchRemoteServerTools lists the tools of one remote MCP server.
func (c *Client) fetchRemoteServerTools(orgID, serverUUID string) ([]remoteMCPTool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	toolsURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers/%s/tools", orgID, serverUUID)
	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("accept", "application/json").
		SetHeader("referer", "https://claude.ai/chat").
		Get(toolsURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %d", resp.StatusCode)
	}
	var data remoteMCPToolsResponse
	if err := json.Unmarshal(resp.Bytes(), &data); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return data.Tools, nil
}

// Live permission states reported by ListMCPTools besides always_allow and
// use_tool.
const (
	mcpModeUnset    = "unset"    // no entry for the tool in enabled_mcp_tools
	mcpModeDisabled = "disabled" // the tool is explicitly switched off
)

// MCPToolStatus is one tool of a connected MCP server with the permission it
// currently has on claude.ai.
type MCPToolStatus struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Mode is always_allow, use_tool, disabled or unset.
	Mode string `json:"mode"`
}

// MCPServerStatus is one connected MCP server and its tools.
type MCPServerStatus struct {
	UUID  string          `json:"uuid"`
	Name  string          `json:"name"`
	URL   string          `json:"url"`
	Tools []MCPToolStatus `json:"tools"`
	Error string          `json:"error,omitempty"`
}

// mcpToolLiveMode reads a tool's current permission from enabled_mcp_tools.
func mcpToolLiveMode(enabled map[string]interface{}, serverUUID string, tool remoteMCPTool) string {
	enabledKey := tool.EnabledKey
	if enabledKey == "" {
		enabledKey = fmt.Sprintf("%s:%s", serverUUID, tool.Name)
	}
	v, ok := enabled[enabledKey]
	if !ok || v == nil {
		return mcpModeUnset
	}
	if v != true {
		return mcpModeDisabled
	}
	if tool.AlwaysApprovedKey != "" {
		if a, ok := enabled[tool.AlwaysApprovedKey]; ok && a == true {
			return config.MCPModeAlwaysAllow
		}
	}
	return config.MCPModeUseTool
}

// ListMCPTools returns the custom remote MCP servers of the account with each
// tool's current permission.
func (c *Client) ListMCPTools() ([]MCPServerStatus, error) {
	orgID, err := c.GetOrgID()
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	servers, err := c.listRemoteServers(orgID)
	if err != nil {
		return nil, err
	}
	enabled, err := c.fetchEnabledMCPTools()
	if err != nil {
		return nil, fmt.Errorf("read account settings failed: %w", err)
	}

	out := make([]MCPServerStatus, 0, len(servers))
	for _, s := range servers {
		if s.UUID == "" {
			continue
		}
		st := MCPServerStatus{UUID: s.UUID, Name: s.Name, URL: s.URL, Tools: []MCPToolStatus{}}
		tools, tErr := c.fetchRemoteServerTools(orgID, s.UUID)
		if tErr != nil {
			st.Error = tErr.Error()
			out = append(out, st)
			continue
		}
		for _, t := range tools {
			if t.Name == "" {
				continue
			}
			st.Tools = append(st.Tools, MCPToolStatus{
				Name:        t.Name,
				Description: truncateText(t.Description, 200),
				Mode:        mcpToolLiveMode(enabled, s.UUID, t),
			})
		}
		out = append(out, st)
	}
	return out, nil
}

// ApplyMCPToolPermissions re-applies the tool permissions to the connectors
// already on the account, without deleting or re-creating any of them.
func (c *Client) ApplyMCPToolPermissions(perms MCPToolPermissions) (*SyncMCPResult, error) {
	orgID, err := c.GetOrgID()
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	servers, err := c.listRemoteServers(orgID)
	if err != nil {
		return nil, err
	}
	enabled, err := c.fetchEnabledMCPTools()
	if err != nil {
		return nil, fmt.Errorf("read account settings failed: %w", err)
	}

	var total mcpModeResult
	var warnings []string
	serverCount := 0
	for _, s := range servers {
		if s.UUID == "" {
			continue
		}
		serverCount++
		tools, tErr := c.fetchRemoteServerTools(orgID, s.UUID)
		if tErr != nil {
			warnings = append(warnings, fmt.Sprintf("không tải được tool của %s: %v", s.Name, tErr))
			continue
		}
		r := applyMCPToolModes(enabled, s.UUID, tools, perms)
		total.WantTrue = append(total.WantTrue, r.WantTrue...)
		total.WantNotTrue = append(total.WantNotTrue, r.WantNotTrue...)
		total.AlwaysAllow += r.AlwaysAllow
		total.UseTool += r.UseTool
		total.MissingKey += r.MissingKey
	}

	if serverCount == 0 {
		return &SyncMCPResult{Status: "warning", Message: "Không có MCP custom nào để áp quyền tool"}, nil
	}
	toolCount := total.AlwaysAllow + total.UseTool
	if toolCount == 0 {
		msg := "Chưa tải được danh sách tool nên chưa áp quyền"
		if len(warnings) > 0 {
			msg += " (" + strings.Join(warnings, "; ") + ")"
		}
		return &SyncMCPResult{Status: "warning", Message: msg}, nil
	}

	written, err := c.writeEnabledMCPTools(enabled, total.WantTrue, total.WantNotTrue)
	if err != nil {
		logger.Warn("Failed to apply MCP tool permissions for org %s: %v", orgID, err)
		if !written {
			return nil, fmt.Errorf("không áp dụng được quyền tool: %w", err)
		}
		warnings = append(warnings, "đã gửi nhưng chưa xác nhận được: "+err.Error())
	}
	if total.MissingKey > 0 {
		warnings = append(warnings, fmt.Sprintf("%d tool thiếu always_approved_key", total.MissingKey))
	}

	status := "ok"
	msg := fmt.Sprintf("Đã áp quyền cho %d tool (%s)", toolCount, describeMCPModes(total.AlwaysAllow, total.UseTool))
	if len(warnings) > 0 {
		status = "warning"
		msg += " (" + strings.Join(warnings, "; ") + ")"
	}
	return &SyncMCPResult{Status: status, Message: msg, ToolCount: toolCount}, nil
}
