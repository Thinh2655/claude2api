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

type remoteMCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
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

// SyncMCPResult holds the outcome of synchronizing a remote MCP server for an account.
type SyncMCPResult struct {
	Account   string `json:"account"`
	Status    string `json:"status"` // "ok" or "error"
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
		if tErr == nil && tResp.StatusCode == http.StatusOK {
			if jsonErr := json.Unmarshal(tResp.Bytes(), &toolsResp); jsonErr == nil && len(toolsResp.Tools) > 0 {
				break
			}
		}
	}

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

	// 5. Update enabled_mcp_tools setting (Always Allow)
	// Read current account settings to clean up old server keys and merge new ones
	enabledMap := make(map[string]interface{})
	accCtx, accCancel := context.WithTimeout(context.Background(), 10*time.Second)
	accResp, accErr := c.client.R().
		SetContext(accCtx).
		SetHeader("accept", "application/json").
		Get("https://claude.ai/api/account")
	accCancel()
	if accErr == nil && accResp.StatusCode == http.StatusOK {
		var accData map[string]interface{}
		if json.Unmarshal(accResp.Bytes(), &accData) == nil {
			if sett, ok := accData["settings"].(map[string]interface{}); ok {
				if curTools, ok := sett["enabled_mcp_tools"].(map[string]interface{}); ok {
					for k, v := range curTools {
						// Drop keys belonging to deleted old servers
						parts := strings.SplitN(k, ":", 2)
						if len(parts) > 0 && oldUUIDs[parts[0]] {
							continue
						}
						enabledMap[k] = v
					}
				}
			}
		}
	}

	for _, tool := range toolsResp.Tools {
		if tool.Name != "" {
			enabledMap[fmt.Sprintf("%s:%s", newServer.UUID, tool.Name)] = true
		}
	}

	if len(enabledMap) > 0 {
		if err := c.UpdateUserSetting("enabled_mcp_tools", enabledMap); err != nil {
			logger.Warn(fmt.Sprintf("Failed to set always-allow for org %s: %v", orgID, err))
		}
	}

	actionDesc := "thêm MCP mới"
	if reused {
		actionDesc = "tái sử dụng connector hiện có"
	}

	status := "ok"
	msg := fmt.Sprintf("Đã xóa %d server cũ, %s với %d tool (Always Allow)", deletedCount, actionDesc, len(toolsResp.Tools))
	if len(toolsResp.Tools) == 0 {
		status = "warning"
		msg = fmt.Sprintf("Đã xóa %d server cũ, %s nhưng chưa tải được danh sách tool", deletedCount, actionDesc)
	}

	return &SyncMCPResult{
		Status:    status,
		Message:   msg,
		Deleted:   deletedCount,
		ToolCount: len(toolsResp.Tools),
	}, nil
}
