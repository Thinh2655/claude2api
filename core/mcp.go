package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

type remoteMCPServer struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

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
		logger.Warn(fmt.Sprintf("Failed to fetch MCP tools for org %s: %v", orgID, err))
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
				logger.Warn(fmt.Sprintf("Failed to fetch tools for MCP server %s (%s): %v", s.Name, s.UUID, tErr))
				return
			}

			var toolsData remoteMCPToolsResponse
			if err := json.Unmarshal(tResp.Bytes(), &toolsData); err != nil {
				logger.Warn(fmt.Sprintf("Failed to unmarshal tools for MCP server %s: %v", s.Name, err))
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
	logger.Info(fmt.Sprintf("Fetched %d MCP tools for org %s", len(results), orgID))
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
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s, ""
}

// SyncMCPResult holds the outcome of an MCP sync/delete operation for an account.
type SyncMCPResult struct {
	Account   string `json:"account"`
	Status    string `json:"status"` // "ok", "warning" or "error"
	Message   string `json:"message"`
	Deleted   int    `json:"deleted"`
	ToolCount int    `json:"toolCount"`
}

// SyncRemoteMCP replaces existing custom remote MCP servers on the account with the new MCP URL and name,
// then marks all its tools as always allowed (enabled_mcp_tools).
func (c *Client) SyncRemoteMCP(mcpName, mcpURL string) (*SyncMCPResult, error) {
	orgID, err := c.GetOrgID()
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	mcpName = strings.TrimSpace(mcpName)
	if mcpName == "" {
		mcpName = "mcp"
	}
	if len(mcpName) > 64 {
		mcpName = mcpName[:64]
	}

	// 1. Fetch current remote servers to know what to delete
	serversURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers", orgID)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("accept", "application/json").
		SetHeader("referer", "https://claude.ai/chat").
		Get(serversURL)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing servers: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list existing servers status: %d", resp.StatusCode)
	}

	var oldServers []remoteMCPServer
	if err := json.Unmarshal(resp.Bytes(), &oldServers); err != nil {
		return nil, fmt.Errorf("failed to parse existing servers: %w", err)
	}

	normTargetURL := strings.TrimRight(strings.ToLower(mcpURL), "/")
	var newServer remoteMCPServer
	reused := false

	// Check if a connector with the exact same URL already exists in oldServers
	for _, s := range oldServers {
		if strings.TrimRight(strings.ToLower(s.URL), "/") == normTargetURL && s.UUID != "" {
			newServer = s
			reused = true
			logger.Info(fmt.Sprintf("Found existing connector %s matching URL %s", s.UUID, mcpURL))
			// If existing connector has different name, update name via PUT
			if s.Name != mcpName {
				putURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers/%s", orgID, s.UUID)
				putBody := map[string]interface{}{"name": mcpName}
				c.client.R().
					SetHeader("accept", "application/json").
					SetHeader("content-type", "application/json").
					SetHeader("referer", "https://claude.ai/chat").
					SetBody(putBody).
					Put(putURL)
				newServer.Name = mcpName
			}
			break
		}
	}

	// 2. If not found in existing list, create the new remote server
	if !reused {
		createBody := map[string]interface{}{
			"name":          mcpName,
			"url":           mcpURL,
			"id_jag_config": nil,
			"attestations":  []interface{}{},
		}
		createCtx, createCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer createCancel()

		createResp, err := c.client.R().
			SetContext(createCtx).
			SetHeader("accept", "application/json").
			SetHeader("content-type", "application/json").
			SetHeader("referer", "https://claude.ai/chat").
			SetBody(createBody).
			Post(serversURL)
		if err != nil {
			return nil, fmt.Errorf("failed to create new MCP server: %w", err)
		}

		if createResp.StatusCode == http.StatusConflict {
			// 409 Conflict: A connector with this URL already exists
			errMsg, existingUUID := parseUpstreamError(createResp.Bytes())
			if existingUUID != "" {
				newServer.UUID = existingUUID
				newServer.Name = mcpName
				reused = true
				logger.Info(fmt.Sprintf("409 Conflict: reusing existing connector UUID %s for URL %s", existingUUID, mcpURL))
				// Try updating name
				putURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers/%s", orgID, existingUUID)
				putBody := map[string]interface{}{"name": mcpName}
				c.client.R().
					SetHeader("accept", "application/json").
					SetHeader("content-type", "application/json").
					SetHeader("referer", "https://claude.ai/chat").
					SetBody(putBody).
					Put(putURL)
			} else {
				return nil, fmt.Errorf("URL MCP này đã tồn tại trên tài khoản (409 Conflict: %s). Vui lòng kiểm tra lại Connectors trên claude.ai", errMsg)
			}
		} else if createResp.StatusCode != http.StatusOK && createResp.StatusCode != http.StatusCreated {
			errMsg, _ := parseUpstreamError(createResp.Bytes())
			return nil, fmt.Errorf("tạo MCP server thất bại (status %d): %s", createResp.StatusCode, errMsg)
		} else {
			if err := json.Unmarshal(createResp.Bytes(), &newServer); err != nil {
				return nil, fmt.Errorf("failed to parse created MCP server response: %w", err)
			}
			if newServer.UUID == "" {
				return nil, errors.New("created MCP server returned empty UUID")
			}
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
	deletedCount := 0
	oldUUIDs := make(map[string]bool)
	for _, old := range oldServers {
		if old.UUID == "" || old.UUID == newServer.UUID {
			continue
		}
		oldUUIDs[old.UUID] = true
		delURL := fmt.Sprintf("https://claude.ai/api/organizations/%s/mcp/remote_servers/%s", orgID, old.UUID)
		dCtx, dCancel := context.WithTimeout(context.Background(), 10*time.Second)
		dResp, dErr := c.client.R().
			SetContext(dCtx).
			SetHeader("accept", "application/json").
			SetHeader("referer", "https://claude.ai/chat").
			Delete(delURL)
		dCancel()
		if dErr == nil && (dResp.StatusCode == http.StatusOK || dResp.StatusCode == http.StatusNoContent) {
			deletedCount++
		} else {
			logger.Warn(fmt.Sprintf("Failed to delete old MCP server %s: %v", old.UUID, dErr))
		}
	}

	// 5. Update enabled_mcp_tools setting (Always Allow).
	// claude.ai stores two keys per tool: "<uuid>:<tool>" (tool enabled) and
	// "<uuid>:<tool>-<hash>" (Always Allow). Both must be true so the upstream
	// call does not pause for manual approval mid-stream.
	enabledMap, settingsErr := c.fetchEnabledMCPTools()
	if settingsErr != nil {
		logger.Warn(fmt.Sprintf("Failed to read enabled_mcp_tools for org %s: %v", orgID, settingsErr))
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

	missingAlways := 0
	alwaysKeys := make([]string, 0, len(toolsResp.Tools))
	if settingsErr == nil {
		for _, tool := range toolsResp.Tools {
			if tool.Name == "" {
				continue
			}
			enabledKey := tool.EnabledKey
			if enabledKey == "" {
				enabledKey = fmt.Sprintf("%s:%s", newServer.UUID, tool.Name)
			}
			enabledMap[enabledKey] = true
			if tool.AlwaysApprovedKey == "" {
				missingAlways++
				continue
			}
			enabledMap[tool.AlwaysApprovedKey] = true
			alwaysKeys = append(alwaysKeys, tool.AlwaysApprovedKey)
		}
	}

	settingErr := settingsErr
	if settingErr == nil && len(toolsResp.Tools) > 0 {
		if err := c.UpdateUserSetting("enabled_mcp_tools", enabledMap); err != nil {
			settingErr = fmt.Errorf("write setting: %w", err)
			logger.Warn(fmt.Sprintf("Failed to set always-allow for org %s: %v", orgID, err))
		} else {
			// PATCH returns 202 (accepted), so allow the read-back a moment to
			// catch up before reporting a failure.
			missingKey := ""
			for attempt := 0; attempt < 3; attempt++ {
				if attempt > 0 {
					time.Sleep(500 * time.Millisecond)
				}
				saved, err := c.fetchEnabledMCPTools()
				if err != nil {
					settingErr = fmt.Errorf("verify setting: %w", err)
					logger.Warn(fmt.Sprintf("Failed to verify always-allow for org %s: %v", orgID, err))
					continue
				}
				settingErr = nil
				missingKey = ""
				for _, key := range alwaysKeys {
					if v, ok := saved[key]; !ok || v != true {
						missingKey = key
						break
					}
				}
				if missingKey == "" {
					break
				}
			}
			if settingErr == nil && missingKey != "" {
				settingErr = fmt.Errorf("setting %q was not persisted", missingKey)
				logger.Warn(fmt.Sprintf("Always Allow key %q missing after sync for org %s", missingKey, orgID))
			}
		}
	}

	actionDesc := "thêm MCP mới"
	if reused {
		actionDesc = "tái sử dụng connector hiện có"
	}

	status := "ok"
	msg := fmt.Sprintf("Đã xóa %d server cũ, %s với %d tool (Always Allow)", deletedCount, actionDesc, len(toolsResp.Tools))
	var warnings []string
	if len(toolsResp.Tools) == 0 {
		status = "warning"
		msg = fmt.Sprintf("Đã xóa %d server cũ, %s nhưng chưa tải được danh sách tool nên chưa bật Always Allow", deletedCount, actionDesc)
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
			alwaysOK := settingErr == nil && missingAlways == 0
			switch {
			case state == "null" && len(toolsResp.Tools) > 0 && alwaysOK:
				detail += " - gọi tool qua API thường vẫn hoạt động; nếu dùng UI hãy Connect trên claude.ai"
			case state == "null":
				detail += " - cần Connect trên claude.ai hoặc kiểm tra MCP server"
			case state == "error" && auth != "" && auth != "authless":
				detail += " - sai loại auth hoặc cần đăng nhập lại"
			default:
				detail += " - cần Connect trên claude.ai hoặc MCP server đang down"
			}
			if len(conn.Warnings) > 0 {
				w := strings.Join(conn.Warnings, "; ")
				if len(w) > 200 {
					w = w[:200] + "..."
				}
				detail += ": " + w
			}
			warnings = append(warnings, detail)
		}
	}
	if missingAlways > 0 {
		warnings = append(warnings, fmt.Sprintf("%d tool thiếu always_approved_key", missingAlways))
	}
	if settingErr != nil && len(toolsResp.Tools) > 0 {
		warnings = append(warnings, "chưa bật được Always Allow")
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
			logger.Warn(fmt.Sprintf("Failed to delete MCP server %s (%s): %v", s.Name, s.UUID, err))
			continue
		}
		deleted[s.UUID] = true
	}

	settingWarn := false
	if len(deleted) > 0 {
		if err := c.pruneEnabledMCPTools(deleted); err != nil {
			settingWarn = true
			logger.Warn(fmt.Sprintf("Failed to prune enabled_mcp_tools for org %s: %v", orgID, err))
		}
	}

	// Re-list to confirm nothing was left behind (catches pagination or a
	// DELETE that returned success without actually removing the server).
	remaining := 0
	verified := true
	if after, listErr := c.listRemoteServers(orgID); listErr != nil {
		verified = false
		logger.Warn(fmt.Sprintf("Failed to verify remaining MCP servers for org %s: %v", orgID, listErr))
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
