package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
