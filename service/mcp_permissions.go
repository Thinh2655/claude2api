package service

import (
	"claude2api/config"
	"claude2api/core"
	"claude2api/logger"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// resolveMCPPermissions merges the permission fields of a request with the
// stored ones. An empty mode keeps the stored default mode and a nil modes map
// keeps the stored per-tool overrides (an empty, non-nil map clears them).
func resolveMCPPermissions(mode string, modes map[string]string) (string, map[string]string, error) {
	curMode, curModes := config.ConfigInstance.MCPToolPermissions()
	if strings.TrimSpace(mode) != "" {
		m, ok := config.NormalizeMCPToolMode(mode)
		if !ok {
			return "", nil, fmt.Errorf("toolMode không hợp lệ: %q (dùng always_allow hoặc use_tool)", mode)
		}
		curMode = m
	}
	if modes != nil {
		valid, err := config.ValidateMCPToolModes(modes)
		if err != nil {
			return "", nil, err
		}
		curModes = valid
	}
	return curMode, curModes, nil
}

// saveMCPPermissions stores the MCP tool permissions in config and persists
// them to .env (MCP_TOOL_MODE, MCP_TOOL_MODES) for the next start.
func saveMCPPermissions(mode string, modes map[string]string) {
	config.ConfigInstance.SetMCPToolPermissions(mode, modes)
	if err := saveEnvLine("MCP_TOOL_MODE", mode); err != nil {
		logger.Error("Failed to persist MCP_TOOL_MODE to .env: %v", err)
	}
	if err := saveEnvLine("MCP_TOOL_MODES", "'"+config.EncodeMCPToolModes(modes)+"'"); err != nil {
		logger.Error("Failed to persist MCP_TOOL_MODES to .env: %v", err)
	}
}

// MCPToolsHandler lists the MCP servers of one account with each tool's
// current permission on claude.ai. Query: ?key=<full session key> picks the
// account; without it the first enabled account of the pool is used.
func MCPToolsHandler(c *gin.Context) {
	key := strings.TrimSpace(c.Query("key"))

	var sess config.SessionInfo
	found := false
	config.ConfigInstance.RwMutx.RLock()
	for _, s := range config.ConfigInstance.Sessions {
		if key != "" {
			if s.SessionKey == key {
				sess, found = s, true
				break
			}
			continue
		}
		if !s.Disabled {
			sess, found = s, true
			break
		}
	}
	config.ConfigInstance.RwMutx.RUnlock()
	if !found {
		if key != "" {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Account not found"})
			return
		}
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Không có tài khoản nào đang bật trong pool"})
		return
	}

	client := core.NewClientWithCookie(sess.SessionKey, config.ConfigInstance.Proxy, "", sess.ExtraCookie, sess.DeviceID)
	servers, err := client.ListMCPTools()
	if err != nil {
		logger.Warn("MCP tool list failed for account %s: %v", displayLabel(&sess), err)
		c.JSON(http.StatusBadGateway, ErrorResponse{Error: err.Error()})
		return
	}

	mode, modes := config.ConfigInstance.MCPToolPermissions()
	c.JSON(http.StatusOK, gin.H{
		"account":   displayLabel(&sess),
		"servers":   servers,
		"toolMode":  mode,
		"toolModes": modes,
	})
}

// MCPPermissionsHandler saves the MCP tool permissions (default mode plus
// per-tool overrides) and applies them to every account in the pool without
// deleting or re-creating any connector.
// Body: {"toolMode": "always_allow|use_tool", "toolModes": {"<tool>": "<mode>"}}.
// Both fields are optional; an omitted field keeps its stored value.
func MCPPermissionsHandler(c *gin.Context) {
	var body struct {
		ToolMode  string            `json:"toolMode"`
		ToolModes map[string]string `json:"toolModes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"toolMode\": \"always_allow|use_tool\", \"toolModes\": {\"<tool>\": \"<mode>\"}}"})
		return
	}
	mode, modes, err := resolveMCPPermissions(body.ToolMode, body.ToolModes)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	if !mcpOpMu.TryLock() {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "Đang có thao tác MCP khác chạy, vui lòng thử lại sau"})
		return
	}
	defer mcpOpMu.Unlock()

	saveMCPPermissions(mode, modes)

	config.ConfigInstance.RwMutx.RLock()
	sessionsCopy := make([]config.SessionInfo, len(config.ConfigInstance.Sessions))
	copy(sessionsCopy, config.ConfigInstance.Sessions)
	config.ConfigInstance.RwMutx.RUnlock()

	if len(sessionsCopy) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "Không có tài khoản nào trong pool",
			"results": []core.SyncMCPResult{},
		})
		return
	}

	perms := core.MCPToolPermissions{Default: mode, Tools: modes}
	results := make([]core.SyncMCPResult, len(sessionsCopy))
	var wg sync.WaitGroup
	// Limit concurrency to 3 to avoid hitting Claude rate limits or TLS connection surges
	sem := make(chan struct{}, 3)

	for i, s := range sessionsCopy {
		wg.Add(1)
		go func(idx int, sess config.SessionInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			label := displayLabel(&sess)
			client := core.NewClientWithCookie(sess.SessionKey, config.ConfigInstance.Proxy, "", sess.ExtraCookie, sess.DeviceID)
			res, applyErr := client.ApplyMCPToolPermissions(perms)
			if applyErr != nil {
				results[idx] = core.SyncMCPResult{
					Account: label,
					Status:  "error",
					Message: applyErr.Error(),
				}
				logger.Warn("MCP permission apply failed for account %s: %v", label, applyErr)
				return
			}
			res.Account = label
			results[idx] = *res
			logger.Info("MCP permission apply for account %s: %s", label, res.Message)
		}(i, s)
	}

	wg.Wait()

	c.JSON(http.StatusOK, gin.H{
		"message":   "Đã hoàn thành áp dụng quyền tool MCP",
		"toolMode":  mode,
		"toolModes": modes,
		"results":   results,
	})
}
