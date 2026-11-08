package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"claude2api/config"
	"claude2api/logger"
)

type mcpCacheEntry struct {
	tools     []map[string]interface{}
	expiresAt time.Time
}

var (
	mcpCacheMu sync.RWMutex
	mcpCache   = make(map[string]mcpCacheEntry)
	mcpFetchMu sync.Map // per-org singleflight lock
)

type remoteMCPInstallSummary struct {
	Transport          string  `json:"transport"`
	Posture            string  `json:"posture"`
	SelectedMethodType *string `json:"selected_method_type"`
}

type remoteMCPServer struct {
	UUID           string                   `json:"uuid"`
	Name           string                   `json:"name"`
	URL            string                   `json:"url"`
	InstallSummary *remoteMCPInstallSummary `json:"install_summary"`
}

type MCPHeader struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Required bool   `json:"required"`
}

type MCPInstallOptions struct {
	AuthMode     string      `json:"authMode"`
	Registration string      `json:"registration"`
	ClientID     string      `json:"clientId"`
	ClientSecret string      `json:"clientSecret"`
	Headers      []MCPHeader `json:"headers"`
}

type MCPInstall struct {
	Type       string            `json:"type"`
	Transport  string            `json:"transport"`
	Posture    string            `json:"posture"`
	AuthMethod *MCPAuthMethod    `json:"auth_method,omitempty"`
	Inputs     []MCPInstallInput `json:"inputs"`
}

type MCPAuthMethod struct {
	Type              string             `json:"type"`
	Registration      *MCPRegistration   `json:"registration,omitempty"`
	MCPRequestHeaders []MCPRequestHeader `json:"mcp_request_headers"`
}

type MCPRegistration struct {
	Type             string               `json:"type"`
	ClientCredential *MCPClientCredential `json:"client_credential,omitempty"`
}

type MCPClientCredential struct {
	Type string `json:"type"`
}

type MCPRequestHeader struct {
	Name     string `json:"name"`
	Scope    string `json:"scope"`
	Required bool   `json:"required"`
}

type MCPInstallInput struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

var mcpHeaderNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

type remoteMCPConnection struct {
	UUID               string   `json:"uuid"`
	ConnectionState    *string  `json:"connection_state"`
	ConnectionAuthType *string  `json:"connection_auth_type"`
	SignInPending      bool     `json:"sign_in_pending"`
	Warnings           []string `json:"warnings"`
}

type remoteMCPTool struct {
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	InputSchema       map[string]interface{} `json:"inputSchema"`
	EnabledKey        string                 `json:"enabled_key"`
	AlwaysApprovedKey string                 `json:"always_approved_key"`
}

type remoteMCPToolsResponse struct {
	Tools []remoteMCPTool `json:"tools"`
}

// ClearMCPCache clears the in-memory MCP tools cache (e.g. when MCP settings change).
func ClearMCPCache() {
	mcpCacheMu.Lock()
	mcpCache = make(map[string]mcpCacheEntry)
	mcpCacheMu.Unlock()
}

// AttachMCPTools fetches configured remote MCP tools for the current organization
// and appends them to the upstream tools list if EnableMCP is enabled.
func (c *Client) AttachMCPTools() {
	if !config.ConfigInstance.MCPEnabled() || c.orgID == "" {
		return
	}

	mcpTools := c.getOrFetchMCPTools(c.orgID)
	if len(mcpTools) == 0 {
		return
	}

	currentTools, _ := c.defaultAttrs["tools"].([]map[string]interface{})
	combined := make([]map[string]interface{}, 0, len(currentTools)+len(mcpTools))
	combined = append(combined, currentTools...)
	combined = append(combined, mcpTools...)
	c.defaultAttrs["tools"] = combined
}

func (c *Client) getOrFetchMCPTools(orgID string) []map[string]interface{} {
	now := time.Now()

	// 1. Check in-memory cache first
	mcpCacheMu.RLock()
	entry, exists := mcpCache[orgID]
	mcpCacheMu.RUnlock()

	if exists && now.Before(entry.expiresAt) {
		return entry.tools
	}

	// 2. Fetch with per-org lock to avoid dogpiling
	lockVal, _ := mcpFetchMu.LoadOrStore(orgID, &sync.Mutex{})
	lock := lockVal.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	// Double check cache under lock
	mcpCacheMu.RLock()
	entry, exists = mcpCache[orgID]
	mcpCacheMu.RUnlock()
	if exists && now.Before(entry.expiresAt) {
		return entry.tools
	}

	tools, err := c.fetchRemoteMCPTools(orgID)
	ttl := 15 * time.Minute
	if err != nil {
		logger.Warn("Failed to fetch MCP tools for org %s: %v", orgID, err)
		// If we had stale tools, keep using them on error
		if exists && len(entry.tools) > 0 {
			ttl = 2 * time.Minute
			tools = entry.tools
		} else {
			// Cache empty tools briefly to avoid hitting upstream repeatedly on outage
			ttl = 1 * time.Minute
			tools = nil
		}
	}

	mcpCacheMu.Lock()
	mcpCache[orgID] = mcpCacheEntry{
		tools:     tools,
		expiresAt: time.Now().Add(ttl),
	}
	mcpCacheMu.Unlock()

	return tools
}

func (c *Client) fetchRemoteMCPTools(orgID string) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serversURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers", orgID)
	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("accept", "application/json").
		SetHeader("referer", "https://claude.ai/chat").
		Get(serversURL)
	if err != nil {
		return nil, fmt.Errorf("fetch remote servers failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch remote servers status: %d", resp.StatusCode)
	}

	var servers []remoteMCPServer
	if err := json.Unmarshal(resp.Bytes(), &servers); err != nil {
		return nil, fmt.Errorf("parse remote servers response failed: %w", err)
	}

	if len(servers) == 0 {
		return nil, nil
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []map[string]interface{}
	)

	for _, srv := range servers {
		if srv.UUID == "" {
			continue
		}
		wg.Add(1)
		go func(s remoteMCPServer) {
			defer wg.Done()
			toolCtx, toolCancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer toolCancel()

			toolsURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers/%s/tools", orgID, s.UUID)
			tResp, tErr := c.client.R().
				SetContext(toolCtx).
				SetHeader("accept", "application/json").
				SetHeader("referer", "https://claude.ai/chat").
				Get(toolsURL)
			if tErr != nil || tResp.StatusCode != http.StatusOK {
				logger.Warn("Failed to fetch tools for MCP server %s (%s): %v", s.Name, s.UUID, tErr)
				return
			}

			var toolsData remoteMCPToolsResponse
			if err := json.Unmarshal(tResp.Bytes(), &toolsData); err != nil {
				logger.Warn("Failed to unmarshal tools for MCP server %s: %v", s.Name, err)
				return
			}

			var serverTools []map[string]interface{}
			for _, t := range toolsData.Tools {
				if t.Name == "" {
					continue
				}
				schema := t.InputSchema
				if schema == nil {
					schema = map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					}
				}
				serverTools = append(serverTools, map[string]interface{}{
					"name":              t.Name,
					"description":       t.Description,
					"input_schema":      schema,
					"integration_name":  s.Name,
					"mcp_server_uuid":   s.UUID,
					"mcp_server_url":    s.URL,
					"backend_execution": true,
				})
			}

			if len(serverTools) > 0 {
				mu.Lock()
				results = append(results, serverTools...)
				mu.Unlock()
			}
		}(srv)
	}

	wg.Wait()
	logger.Info("Fetched %d MCP tools for org %s", len(results), orgID)
	return results, nil
}

// parseUpstreamError extracts a human-readable message and an optional existing_connector_uuid
// from a claude.ai error JSON response.
func parseUpstreamError(body []byte) (message, existingUUID string) {
	var e struct {
		Error struct {
			Message string `json:"message"`
			Details struct {
				ExistingConnectorUUID string `json:"existing_connector_uuid"`
			} `json:"details"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error.Message != "" {
		return e.Error.Message, e.Error.Details.ExistingConnectorUUID
	}
	s := truncateText(strings.TrimSpace(string(body)), 200)
	return s, ""
}

func truncateText(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "..."
}

// SyncMCPResult holds the outcome of an MCP sync/delete operation for an account.
type SyncMCPResult struct {
	Account   string `json:"account"`
	Status    string `json:"status"` // "ok", "warning" or "error"
	Message   string `json:"message"`
	Deleted   int    `json:"deleted"`
	ToolCount int    `json:"toolCount"`
}

func BuildMCPInstall(mcpURL string, opts MCPInstallOptions) (*MCPInstall, error) {
	authMode := strings.TrimSpace(opts.AuthMode)
	if authMode == "" {
		authMode = "no_auth"
	}
	switch authMode {
	case "no_auth", "defer_auth", "auth_required":
	default:
		return nil, fmt.Errorf("authMode không hợp lệ: %q", authMode)
	}

	registration := strings.TrimSpace(opts.Registration)
	clientID := strings.TrimSpace(opts.ClientID)
	clientSecret := strings.TrimSpace(opts.ClientSecret)
	if authMode == "no_auth" && (registration != "" || clientID != "" || clientSecret != "") {
		return nil, errors.New("registration/clientId/clientSecret chỉ dùng khi bật OAuth")
	}
	if authMode != "no_auth" {
		switch registration {
		case "cimd", "dcr", "static_per_installation":
		case "":
			return nil, errors.New("registration là bắt buộc khi dùng OAuth")
		default:
			return nil, fmt.Errorf("registration không hợp lệ: %q", registration)
		}
		if registration != "static_per_installation" && (clientID != "" || clientSecret != "") {
			return nil, errors.New("clientId/clientSecret chỉ dùng với registration=static_per_installation")
		}
		if registration == "static_per_installation" && clientID == "" {
			return nil, errors.New("clientId là bắt buộc khi registration=static_per_installation")
		}
	}

	headers := make([]MCPHeader, 0, len(opts.Headers))
	for _, raw := range opts.Headers {
		name := strings.TrimSpace(raw.Name)
		value := strings.TrimSpace(raw.Value)
		if name == "" {
			if value != "" {
				return nil, errors.New("tên request header không được để trống")
			}
			continue
		}
		if len(headers) >= 4 {
			return nil, errors.New("tối đa 4 request header")
		}
		if !validMCPHeaderName(name) {
			return nil, fmt.Errorf("tên request header không hợp lệ: %q", name)
		}
		if !validMCPHeaderValue(value) {
			return nil, fmt.Errorf("giá trị request header không hợp lệ: %q", name)
		}
		if raw.Required && value == "" {
			return nil, fmt.Errorf("request header %q bắt buộc phải có value", name)
		}
		if authMode != "no_auth" && strings.EqualFold(name, "authorization") {
			return nil, errors.New("header Authorization do OAuth quản lý, không khai báo trong request headers")
		}
		for _, prev := range headers {
			if mcpHeaderNamesAlike(prev.Name, name) {
				return nil, fmt.Errorf("request header bị trùng: %q", name)
			}
		}
		headers = append(headers, MCPHeader{Name: name, Value: value, Required: raw.Required})
	}

	if authMode == "no_auth" && len(headers) > 0 {
		hasValue := false
		for _, h := range headers {
			if h.Value != "" {
				hasValue = true
				break
			}
		}
		if !hasValue {
			return nil, errors.New("khi không dùng OAuth, cần ít nhất một request header có value")
		}
	}

	definitions := make([]MCPRequestHeader, 0, len(headers))
	values := make([]MCPInstallInput, 0, len(headers))
	for _, h := range headers {
		definitions = append(definitions, MCPRequestHeader{
			Name:     h.Name,
			Scope:    "organization",
			Required: h.Required,
		})
		if h.Value != "" {
			values = append(values, MCPInstallInput{
				Type:  "mcp_request_header",
				Name:  h.Name,
				Value: h.Value,
			})
		}
	}

	install := &MCPInstall{
		Type:      "custom",
		Transport: mcpTransportForURL(mcpURL),
		Inputs:    make([]MCPInstallInput, 0),
	}

	if authMode == "no_auth" {
		if len(definitions) == 0 {
			install.Posture = "no_auth"
			return install, nil
		}
		install.Posture = "auth_required"
		install.AuthMethod = &MCPAuthMethod{
			Type:              "static_headers",
			MCPRequestHeaders: definitions,
		}
		install.Inputs = values
		return install, nil
	}

	install.Posture = authMode
	authMethod := &MCPAuthMethod{
		Type: "oauth",
		Registration: &MCPRegistration{
			Type: registration,
		},
		MCPRequestHeaders: definitions,
	}
	if registration == "static_per_installation" {
		credentialType := "none"
		if clientSecret != "" {
			credentialType = "client_secret"
		}
		authMethod.Registration.ClientCredential = &MCPClientCredential{Type: credentialType}
		install.Inputs = append(install.Inputs, MCPInstallInput{
			Type:  "oauth_method_input",
			Name:  "client_id",
			Value: clientID,
		})
		if clientSecret != "" {
			install.Inputs = append(install.Inputs, MCPInstallInput{
				Type:  "oauth_client_credential",
				Name:  "client_secret",
				Value: clientSecret,
			})
		}
	}
	install.Inputs = append(install.Inputs, values...)
	install.AuthMethod = authMethod
	return install, nil
}

func validMCPHeaderName(name string) bool {
	if !mcpHeaderNamePattern.MatchString(name) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] != '.' {
			continue
		}
		if i == 0 || i == len(name)-1 || !isASCIIAlphaNum(name[i-1]) || !isASCIIAlphaNum(name[i+1]) {
			return false
		}
	}
	return true
}

func isASCIIAlphaNum(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

func validMCPHeaderValue(value string) bool {
	if len(value) > 4096 {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func mcpHeaderNamesAlike(a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	if !strings.Contains(a, ".") && !strings.Contains(b, ".") {
		return false
	}
	normalize := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, ".", "-")
		return strings.ReplaceAll(s, "_", "-")
	}
	return normalize(a) == normalize(b)
}

func mcpTransportForURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err == nil && strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/sse") {
		return "sse"
	}
	return "streamable-http"
}

func normalizeMCPURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimRight(strings.TrimSpace(raw), "/")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String()
}

func mcpServerMatchesDefaultInstall(server remoteMCPServer, install *MCPInstall) bool {
	if server.InstallSummary == nil {
		return true
	}
	summary := server.InstallSummary
	return summary.Transport == install.Transport &&
		summary.Posture == "no_auth" &&
		(summary.SelectedMethodType == nil || *summary.SelectedMethodType == "")
}

// SyncRemoteMCP replaces existing custom remote MCP servers on the account with the new MCP URL and name,
// then applies the tool permissions (enabled_mcp_tools) from perms: Always Allow or "use tool" per tool.
func (c *Client) SyncRemoteMCP(mcpName, mcpURL string, install *MCPInstall, perms MCPToolPermissions) (*SyncMCPResult, error) {
	orgID, err := c.GetOrgID()
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	mcpName = strings.TrimSpace(mcpName)
	if mcpName == "" {
		mcpName = "mcp"
	}
	if runes := []rune(mcpName); len(runes) > 64 {
		mcpName = string(runes[:64])
	}

	if install == nil {
		install, err = BuildMCPInstall(mcpURL, MCPInstallOptions{})
		if err != nil {
			return nil, err
		}
	}

	serversURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers", orgID)
	oldServers, err := c.listRemoteServers(orgID)
	if err != nil {
		return nil, err
	}

	normTargetURL := normalizeMCPURL(mcpURL)
	wantReuse := install.Posture == "no_auth" && install.AuthMethod == nil && len(install.Inputs) == 0
	var newServer remoteMCPServer
	reused := false
	renameWarn := ""

	if wantReuse {
		for _, s := range oldServers {
			if s.UUID == "" || normalizeMCPURL(s.URL) != normTargetURL || !mcpServerMatchesDefaultInstall(s, install) {
				continue
			}
			newServer = s
			reused = true
			logger.Info("Found existing connector %s matching URL %s", s.UUID, mcpURL)
			if s.Name != mcpName {
				if err := c.renameRemoteServer(orgID, s.UUID, mcpName); err != nil {
					renameWarn = fmt.Sprintf("không đổi được tên connector cũ: %v", err)
				} else {
					newServer.Name = mcpName
				}
			}
			break
		}
	}

	deletedCount := 0
	deletedUUIDs := make(map[string]bool)

	if !reused {
		createBody := map[string]interface{}{
			"name":         mcpName,
			"url":          mcpURL,
			"install":      install,
			"attestations": []interface{}{},
		}
		status, body, err := c.postRemoteMCPServer(serversURL, createBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create new MCP server: %w", err)
		}

		if status == http.StatusConflict {
			errMsg, existingUUID := parseUpstreamError(body)
			if existingUUID == "" {
				for _, s := range oldServers {
					if s.UUID != "" && normalizeMCPURL(s.URL) == normTargetURL {
						existingUUID = s.UUID
						break
					}
				}
			}
			if existingUUID == "" {
				return nil, fmt.Errorf("URL MCP này đã tồn tại trên tài khoản (409 Conflict: %s). Vui lòng kiểm tra lại Connectors trên claude.ai", errMsg)
			}
			if err := c.deleteRemoteServer(orgID, existingUUID); err != nil {
				return nil, fmt.Errorf("connector cùng URL đã tồn tại nhưng không xóa được để tạo lại (%s): %w", existingUUID, err)
			}
			deletedCount++
			deletedUUIDs[existingUUID] = true
			logger.Info("409 Conflict: deleted connector %s, retrying create for URL %s", existingUUID, mcpURL)
			time.Sleep(500 * time.Millisecond)
			status, body, err = c.postRemoteMCPServer(serversURL, createBody)
			if err != nil {
				return nil, fmt.Errorf("đã xóa connector cũ nhưng tạo lại thất bại: %w", err)
			}
		}

		if status == http.StatusConflict {
			errMsg, _ := parseUpstreamError(body)
			return nil, fmt.Errorf("URL MCP vẫn tồn tại sau khi xóa connector cũ (409 Conflict: %s)", errMsg)
		}
		if status != http.StatusOK && status != http.StatusCreated {
			errMsg, _ := parseUpstreamError(body)
			return nil, fmt.Errorf("tạo MCP server thất bại (status %d): %s", status, errMsg)
		}
		if err := json.Unmarshal(body, &newServer); err != nil {
			return nil, fmt.Errorf("failed to parse created MCP server response: %w", err)
		}
		if newServer.UUID == "" {
			return nil, errors.New("created MCP server returned empty UUID")
		}
	}

	// 3. Poll for tools from the newly created server (up to 10 seconds)
	var toolsResp remoteMCPToolsResponse
	lastToolsStatus := 0
	var lastToolsErr error
	toolsURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers/%s/tools", orgID, newServer.UUID)
	for i := 0; i < 10; i++ {
		time.Sleep(1 * time.Second)
		tCtx, tCancel := context.WithTimeout(context.Background(), 8*time.Second)
		tResp, tErr := c.client.R().
			SetContext(tCtx).
			SetHeader("accept", "application/json").
			SetHeader("referer", "https://claude.ai/chat").
			Get(toolsURL)
		tCancel()
		lastToolsStatus, lastToolsErr = 0, tErr
		if tErr == nil {
			lastToolsStatus = tResp.StatusCode
			if tResp.StatusCode == http.StatusOK {
				if jsonErr := json.Unmarshal(tResp.Bytes(), &toolsResp); jsonErr == nil && len(toolsResp.Tools) > 0 {
					break
				} else if jsonErr != nil {
					lastToolsErr = jsonErr
				}
			}
		}
	}

	// 3b. Read the connector's connection state. A connector can exist with a
	// cached tool list while claude.ai has no live MCP connection, so the sync
	// result must surface that instead of reporting plain success.
	conn, connErr := c.fetchRemoteServerConnection(orgID, newServer.UUID)

	// 4. Delete old remote servers (all except the newly created one)
	oldUUIDs := make(map[string]bool, len(oldServers)+1)
	for _, old := range oldServers {
		if old.UUID != "" {
			oldUUIDs[old.UUID] = true
		}
	}
	for uuid := range deletedUUIDs {
		oldUUIDs[uuid] = true
	}
	oldUUIDs[newServer.UUID] = true
	for _, old := range oldServers {
		if old.UUID == "" || old.UUID == newServer.UUID || deletedUUIDs[old.UUID] {
			continue
		}
		if err := c.deleteRemoteServer(orgID, old.UUID); err != nil {
			logger.Warn("Failed to delete old MCP server %s: %v", old.UUID, err)
			continue
		}
		deletedCount++
	}

	// 5. Update the enabled_mcp_tools setting (tool permissions).
	// claude.ai stores two keys per tool: "<uuid>:<tool>" (tool enabled) and
	// "<uuid>:<tool>-<hash>" (Always Allow). Always Allow tools get both keys
	// set to true so the upstream call does not pause for manual approval
	// mid-stream; "use_tool" tools only get the first key, so claude.ai asks
	// for approval on each call.
	enabledMap, settingsErr := c.fetchEnabledMCPTools()
	if settingsErr != nil {
		logger.Warn("Failed to read enabled_mcp_tools for org %s: %v", orgID, settingsErr)
	} else {
		// Drop keys of deleted servers and stale keys of the reused server:
		// the always-allow hash can change when a tool definition changes.
		// PATCH merges the map, so set removed keys to nil instead of omitting
		// them; otherwise claude.ai keeps the old value.
		for k := range enabledMap {
			parts := strings.SplitN(k, ":", 2)
			if len(parts) > 0 && (oldUUIDs[parts[0]] || parts[0] == newServer.UUID) {
				enabledMap[k] = nil
			}
		}
	}

	var modeRes mcpModeResult
	if settingsErr == nil {
		modeRes = applyMCPToolModes(enabledMap, newServer.UUID, toolsResp.Tools, perms)
	}
	missingAlways := modeRes.MissingKey

	settingErr := settingsErr
	if settingErr == nil && len(toolsResp.Tools) > 0 {
		if _, err := c.writeEnabledMCPTools(enabledMap, modeRes.WantTrue, modeRes.WantNotTrue); err != nil {
			settingErr = err
			logger.Warn("Failed to set MCP tool permissions for org %s: %v", orgID, err)
		}
	}

	actionDesc := "thêm MCP mới"
	if reused {
		actionDesc = "tái sử dụng connector hiện có"
	} else if install.AuthMethod != nil {
		switch install.AuthMethod.Type {
		case "oauth":
			actionDesc = "thêm MCP mới với OAuth"
		case "static_headers":
			actionDesc = "thêm MCP mới với request headers"
		}
	}

	status := "ok"
	msg := fmt.Sprintf("Đã xóa %d server cũ, %s với %d tool (%s)", deletedCount, actionDesc, len(toolsResp.Tools), describeMCPModes(modeRes.AlwaysAllow, modeRes.UseTool))
	var warnings []string
	if renameWarn != "" {
		warnings = append(warnings, renameWarn)
	}
	if len(toolsResp.Tools) == 0 {
		status = "warning"
		msg = fmt.Sprintf("Đã xóa %d server cũ, %s nhưng chưa tải được danh sách tool nên chưa áp quyền tool", deletedCount, actionDesc)
		if lastToolsStatus != 0 {
			msg += fmt.Sprintf(" (claude.ai /tools trả %d)", lastToolsStatus)
		} else if lastToolsErr != nil {
			msg += fmt.Sprintf(" (%v)", lastToolsErr)
		}
	}
	if connErr != nil {
		warnings = append(warnings, fmt.Sprintf("không đọc được trạng thái kết nối: %v", connErr))
	} else if conn == nil {
		warnings = append(warnings, "không tìm thấy connector khi kiểm tra kết nối")
	} else {
		state := "null"
		if conn.ConnectionState != nil && *conn.ConnectionState != "" {
			state = *conn.ConnectionState
		}
		auth := ""
		if conn.ConnectionAuthType != nil {
			auth = *conn.ConnectionAuthType
		}
		if state != "connected" {
			detail := fmt.Sprintf("connector chưa có kết nối thường trực (state=%s", state)
			if auth != "" {
				detail += ", auth=" + auth
			}
			if conn.SignInPending {
				detail += ", đang chờ đăng nhập"
			}
			detail += ")"
			// With any "use tool" tool the API call would pause for approval,
			// so only claim API calls work when every tool is Always Allow.
			alwaysOK := settingErr == nil && missingAlways == 0 && modeRes.UseTool == 0
			switch {
			case state == "null" && len(toolsResp.Tools) > 0 && alwaysOK:
				detail += " - gọi tool qua API thường vẫn hoạt động; nếu dùng UI hãy Connect trên claude.ai"
			case state == "null":
				detail += " - cần Connect trên claude.ai hoặc kiểm tra MCP server"
			case state == "error" && install.AuthMethod != nil && install.AuthMethod.Type == "static_headers":
				detail += " - kiểm tra request header/API key"
			case state == "error" && auth != "" && auth != "authless":
				detail += " - sai loại auth hoặc cần đăng nhập lại"
			default:
				detail += " - cần Connect trên claude.ai hoặc MCP server đang down"
			}
			if len(conn.Warnings) > 0 {
				w := truncateText(strings.Join(conn.Warnings, "; "), 200)
				detail += ": " + w
			}
			warnings = append(warnings, detail)
		}
	}
	if missingAlways > 0 {
		warnings = append(warnings, fmt.Sprintf("%d tool thiếu always_approved_key", missingAlways))
	}
	if settingErr != nil && len(toolsResp.Tools) > 0 {
		warnings = append(warnings, "chưa áp dụng được quyền tool")
	}
	if len(warnings) > 0 {
		if status == "ok" {
			status = "warning"
		}
		msg += " (" + strings.Join(warnings, "; ") + ")"
	}

	return &SyncMCPResult{
		Status:    status,
		Message:   msg,
		Deleted:   deletedCount,
		ToolCount: len(toolsResp.Tools),
	}, nil
}

func (c *Client) postRemoteMCPServer(serversURL string, body map[string]interface{}) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("accept", "application/json").
		SetHeader("content-type", "application/json").
		SetHeader("referer", "https://claude.ai/chat").
		SetBody(body).
		Post(serversURL)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, resp.Bytes(), nil
}

func (c *Client) renameRemoteServer(orgID, uuid, name string) error {
	putURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers/%s", orgID, uuid)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("accept", "application/json").
		SetHeader("content-type", "application/json").
		SetHeader("referer", "https://claude.ai/chat").
		SetBody(map[string]interface{}{"name": name}).
		Put(putURL)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		errMsg, _ := parseUpstreamError(resp.Bytes())
		return fmt.Errorf("status %d: %s", resp.StatusCode, errMsg)
	}
	return nil
}

// fetchRemoteServerConnection reads one connector's live connection state from
// claude.ai. A nil result means the connector was not found in the response.
func (c *Client) fetchRemoteServerConnection(orgID, uuid string) (*remoteMCPConnection, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("accept", "application/json").
		SetHeader("referer", "https://claude.ai/chat").
		Get(fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers_with_connection", orgID))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %d", resp.StatusCode)
	}

	var servers []remoteMCPConnection
	if err := json.Unmarshal(resp.Bytes(), &servers); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	for i := range servers {
		if servers[i].UUID == uuid {
			return &servers[i], nil
		}
	}
	return nil, nil
}

// listRemoteServers returns the custom remote MCP servers configured on the
// account's organization.
func (c *Client) listRemoteServers(orgID string) ([]remoteMCPServer, error) {
	serversURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers", orgID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("accept", "application/json").
		SetHeader("referer", "https://claude.ai/chat").
		Get(serversURL)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote MCP servers: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list remote MCP servers status: %d", resp.StatusCode)
	}

	var servers []remoteMCPServer
	if err := json.Unmarshal(resp.Bytes(), &servers); err != nil {
		return nil, fmt.Errorf("failed to parse remote MCP servers: %w", err)
	}
	return servers, nil
}

// deleteRemoteServer removes one remote MCP server. A 404 counts as success
// because the server is already gone.
func (c *Client) deleteRemoteServer(orgID, uuid string) error {
	delURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers/%s", orgID, uuid)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("accept", "application/json").
		SetHeader("referer", "https://claude.ai/chat").
		Delete(delURL)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("delete status: %d", resp.StatusCode)
	}
	return nil
}

// fetchEnabledMCPTools reads the account's enabled_mcp_tools map.
func (c *Client) fetchEnabledMCPTools() (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("accept", "application/json").
		Get("https://claude.ai/api/account")
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status: %d", resp.StatusCode)
	}

	var acc struct {
		Settings struct {
			EnabledMCPTools map[string]interface{} `json:"enabled_mcp_tools"`
		} `json:"settings"`
	}
	if err := json.Unmarshal(resp.Bytes(), &acc); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if acc.Settings.EnabledMCPTools == nil {
		acc.Settings.EnabledMCPTools = make(map[string]interface{})
	}
	return acc.Settings.EnabledMCPTools, nil
}

// pruneEnabledMCPTools drops always-allow entries that belong to deleted
// servers. It only writes the setting when something actually changed, so
// accounts without stale entries keep their other settings untouched.
func (c *Client) pruneEnabledMCPTools(deleted map[string]bool) error {
	enabled, err := c.fetchEnabledMCPTools()
	if err != nil {
		return fmt.Errorf("read account settings failed: %w", err)
	}

	kept := make(map[string]interface{}, len(enabled))
	changed := false
	for k, v := range enabled {
		parts := strings.SplitN(k, ":", 2)
		if len(parts) > 0 && deleted[parts[0]] {
			changed = true
			kept[k] = nil
			continue
		}
		kept[k] = v
	}
	if !changed {
		return nil
	}
	return c.UpdateUserSetting("enabled_mcp_tools", kept)
}

// DeleteAllRemoteMCP removes every custom remote MCP server from the account
// and prunes its always-allow entries. The returned result is "ok", "warning"
// (partial deletion / cleanup failure) or "error" (nothing could be deleted).
func (c *Client) DeleteAllRemoteMCP() (*SyncMCPResult, error) {
	orgID, err := c.GetOrgID()
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	servers, err := c.listRemoteServers(orgID)
	if err != nil {
		return nil, err
	}

	total := 0
	for _, s := range servers {
		if s.UUID != "" {
			total++
		}
	}
	if total == 0 {
		return &SyncMCPResult{Status: "ok", Message: "Không có MCP custom nào để xóa"}, nil
	}

	deleted := make(map[string]bool, total)
	failed := 0
	for _, s := range servers {
		if s.UUID == "" {
			continue
		}
		if err := c.deleteRemoteServer(orgID, s.UUID); err != nil {
			failed++
			logger.Warn("Failed to delete MCP server %s (%s): %v", s.Name, s.UUID, err)
			continue
		}
		deleted[s.UUID] = true
	}

	settingWarn := false
	if len(deleted) > 0 {
		if err := c.pruneEnabledMCPTools(deleted); err != nil {
			settingWarn = true
			logger.Warn("Failed to prune enabled_mcp_tools for org %s: %v", orgID, err)
		}
	}

	// Re-list to confirm nothing was left behind (catches pagination or a
	// DELETE that returned success without actually removing the server).
	remaining := 0
	verified := true
	if after, listErr := c.listRemoteServers(orgID); listErr != nil {
		verified = false
		logger.Warn("Failed to verify remaining MCP servers for org %s: %v", orgID, listErr)
	} else {
		for _, s := range after {
			if s.UUID != "" {
				remaining++
			}
		}
	}

	status := "ok"
	msg := fmt.Sprintf("Đã xóa %d MCP custom", len(deleted))
	if len(deleted) == 0 && failed > 0 {
		status = "error"
		msg = fmt.Sprintf("Không xóa được MCP custom nào (%d lỗi)", failed)
	} else if failed > 0 || settingWarn || (verified && remaining > 0) {
		status = "warning"
		if failed > 0 {
			msg += fmt.Sprintf(", %d lỗi khi xóa", failed)
		}
		if verified && remaining > 0 {
			msg += fmt.Sprintf(", còn %d MCP chưa xóa được", remaining)
		}
		if settingWarn {
			msg += "; chưa dọn được quyền Always Allow cũ"
		}
	}

	return &SyncMCPResult{Status: status, Message: msg, Deleted: len(deleted)}, nil
}
