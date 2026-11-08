package service

import (
	"bufio"
	"claude2api/config"
	"claude2api/core"
	"claude2api/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type keyInfo struct {
	Masked      string `json:"masked"`
	Full        string `json:"full,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	HasCookie   bool   `json:"hasCookie"`
	HasOrgID    bool   `json:"hasOrgID"`
	Disabled    bool   `json:"disabled"`
	// Usage-limit lock (acc bị "You've hit your limit"): true khi acc đang bị
	// tạm khóa, kèm thời gian mở khóa + giây còn lại để dashboard đếm ngược.
	Limited          bool   `json:"limited"`
	LimitedUntil     string `json:"limitedUntil,omitempty"`
	LimitedRemaining int64  `json:"limitedRemaining,omitempty"`
	LimitReason      string `json:"limitReason,omitempty"`
	// Cooldown ngắn sau mỗi request (10s success / 30s fail): acc vẫn dùng
	// được nhưng bị đẩy xuống cuối round-robin. Dashboard hiện badge riêng.
	Cooling          bool  `json:"cooling,omitempty"`
	CoolingRemaining int64 `json:"coolingRemaining,omitempty"`
}

type keysResponse struct {
	Keys []keyInfo `json:"keys"`
}

// mcpOpMu serializes MCP sync/delete so two dashboard actions cannot create
// and delete connectors on the same accounts at once.
var mcpOpMu sync.Mutex

// privacySyncMu serializes privacy syncs so two dashboard actions cannot
// overwrite each other's target values on the same accounts.
var privacySyncMu sync.Mutex

// memorySyncMu serializes memory syncs. It is separate from privacySyncMu
// because the settings PATCH merges keys and a rejected memory key must not
// make the privacy sync fail.
var memorySyncMu sync.Mutex

// envFileMu protects the read-modify-write in saveEnvLine from concurrent
// dashboard toggles corrupting .env.
var envFileMu sync.Mutex

type syncResult struct {
	Account string `json:"account"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// maskKey hides the middle of a session key for display
func maskKey(key string) string {
	if len(key) <= 30 {
		return key
	}
	return key[:20] + "..." + key[len(key)-7:]
}

// displayLabel returns the account display name, or the masked key when the
// account has no human-readable label.
func displayLabel(s *config.SessionInfo) string {
	if s.DisplayName != "" {
		return s.DisplayName
	}
	return maskKey(s.SessionKey)
}

// KeysListHandler returns all configured accounts (cookie-based) with a
// display label (account name if set, otherwise a masked key), plus the
// usage-limit lock state (limited + countdown) so the dashboard can show
// temporarily locked accounts.
func KeysListHandler(c *gin.Context) {
	config.ConfigInstance.RwMutx.RLock()
	sessions := make([]struct {
		SessionKey  string
		DisplayName string
		ExtraCookie string
		OrgID       string
		Disabled    bool
	}, len(config.ConfigInstance.Sessions))
	for i := range config.ConfigInstance.Sessions {
		s := &config.ConfigInstance.Sessions[i]
		sessions[i] = struct {
			SessionKey  string
			DisplayName string
			ExtraCookie string
			OrgID       string
			Disabled    bool
		}{s.SessionKey, s.DisplayName, s.ExtraCookie, s.OrgID, s.Disabled}
	}
	config.ConfigInstance.RwMutx.RUnlock()

	resp := keysResponse{Keys: make([]keyInfo, 0, len(sessions))}
	for _, s := range sessions {
		ki := keyInfo{
			Masked:      maskKey(s.SessionKey),
			Full:        s.SessionKey,
			DisplayName: s.DisplayName,
			HasCookie:   s.ExtraCookie != "",
			HasOrgID:    s.OrgID != "",
			Disabled:    s.Disabled,
		}
		if until, rem, reason, limited := config.SessionLimited(s.SessionKey); limited {
			ki.Limited = true
			ki.LimitedUntil = until.Format("15:04:05")
			ki.LimitedRemaining = int64(rem.Seconds())
			ki.LimitReason = reason
		}
		if rem := config.SessionCooldownRemaining(s.SessionKey); rem > 0 {
			ki.Cooling = true
			ki.CoolingRemaining = int64(rem.Seconds()) + 1
		}
		resp.Keys = append(resp.Keys, ki)
	}
	c.JSON(http.StatusOK, resp)
}

// KeyAddHandler adds a new cookie account from a pasted cookie JSON array.
// Body: {"name": "<optional display name>", "cookie": "<JSON array of cookies>"}
func KeyAddHandler(c *gin.Context) {
	var body struct {
		Name   string `json:"name"`
		Cookie string `json:"cookie"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"cookie\": \"[...JSON...]\"}"})
		return
	}
	if strings.TrimSpace(body.Cookie) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide cookie JSON array"})
		return
	}

	var cookies []config.CookieFile
	if err := json.Unmarshal([]byte(body.Cookie), &cookies); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid cookie JSON: %v", err)})
		return
	}

	s, ok := config.ConfigInstance.AddSessionFromCookies(strings.TrimSpace(body.Name), cookies)
	if !ok {
		c.JSON(http.StatusOK, gin.H{"message": "No new account added (missing sessionKey or duplicate)"})
		return
	}
	// No explicit label? Fetch the account email from claude.ai and use it as
	// the display name so the UI shows a human label.
	if s.DisplayName == "" {
		if email := resolveAccountEmail(&s); email != "" {
			config.ConfigInstance.SetSessionDisplayName(s.SessionKey, email)
			s.DisplayName = email
		}
	}
	if err := persistCookieAccounts(); err != nil {
		logger.Error("Failed to persist accounts: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "Added account " + displayLabel(&s),
		"key":     s.SessionKey,
	})
}

// KeyDeleteHandler removes an account by full session key
func KeyDeleteHandler(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Key) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"key\": \"<full session key>\"}"})
		return
	}
	if len(config.ConfigInstance.Sessions) <= 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Cannot delete the last remaining account"})
		return
	}
	if !config.ConfigInstance.RemoveSession(strings.TrimSpace(body.Key)) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Account not found"})
		return
	}
	if err := persistCookieAccounts(); err != nil {
		logger.Error("Failed to persist accounts: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"message": "Account deleted"})
}

// GatewayKeyGetHandler returns the currently selected gateway key
func GatewayKeyGetHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"gatewayKey": config.ConfigInstance.GetGatewayKey()})
}

// GatewayKeySetHandler selects which session key the gateway uses
func GatewayKeySetHandler(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"key\": \"<full session key or empty>\"}"})
		return
	}
	key := strings.TrimSpace(body.Key)
	if key != "" {
		found := false
		config.ConfigInstance.RwMutx.RLock()
		for i := range config.ConfigInstance.Sessions {
			if config.ConfigInstance.Sessions[i].SessionKey == key {
				found = true
				break
			}
		}
		config.ConfigInstance.RwMutx.RUnlock()
		if !found {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Key not found in pool"})
			return
		}
	}
	config.ConfigInstance.SetGatewayKey(key)
	if err := saveEnvLine("GATEWAY_KEY", key); err != nil {
		logger.Error("Failed to persist GATEWAY_KEY to .env: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"message": "Gateway key updated"})
}

// ToolsGetHandler returns whether tool support is enabled
func ToolsGetHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"enableTools": config.ConfigInstance.ToolsEnabled()})
}

// ToolsSetHandler toggles tool support and persists it to .env
func ToolsSetHandler(c *gin.Context) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"enabled\": true|false}"})
		return
	}
	config.ConfigInstance.RwMutx.Lock()
	config.ConfigInstance.EnableTools = body.Enabled
	config.ConfigInstance.RwMutx.Unlock()
	if err := saveEnvLine("ENABLE_TOOLS", fmt.Sprintf("%t", body.Enabled)); err != nil {
		logger.Error("Failed to persist ENABLE_TOOLS to .env: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"enableTools": body.Enabled})
}

// MCPGetHandler returns whether MCP tool support is enabled, plus the stored
// MCP tool permissions (default mode and per-tool overrides).
func MCPGetHandler(c *gin.Context) {
	mode, modes := config.ConfigInstance.MCPToolPermissions()
	c.JSON(http.StatusOK, gin.H{
		"enableMCP": config.ConfigInstance.MCPEnabled(),
		"toolMode":  mode,
		"toolModes": modes,
	})
}

// MCPSetHandler toggles MCP support and persists it to .env
func MCPSetHandler(c *gin.Context) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"enabled\": true|false}"})
		return
	}
	config.ConfigInstance.RwMutx.Lock()
	config.ConfigInstance.EnableMCP = body.Enabled
	config.ConfigInstance.RwMutx.Unlock()
	core.ClearMCPCache()
	if err := saveEnvLine("ENABLE_MCP", fmt.Sprintf("%t", body.Enabled)); err != nil {
		logger.Error("Failed to persist ENABLE_MCP to .env: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"enableMCP": body.Enabled})
}

// PrivacyGetHandler returns the target values used by the privacy sync.
func PrivacyGetHandler(c *gin.Context) {
	geolocation, grove := config.ConfigInstance.PrivacySyncSettings()
	c.JSON(http.StatusOK, gin.H{"geolocation": geolocation, "grove": grove})
}

// PrivacySyncHandler applies the two privacy settings to every account in the
// pool and records the target values in config and .env for the next load.
func PrivacySyncHandler(c *gin.Context) {
	var body struct {
		Geolocation *bool `json:"geolocation"`
		Grove       *bool `json:"grove"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Geolocation == nil || body.Grove == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"geolocation\": true|false, \"grove\": true|false}"})
		return
	}
	geolocation, grove := *body.Geolocation, *body.Grove

	if !privacySyncMu.TryLock() {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "Đang có thao tác đồng bộ quyền riêng tư khác chạy, vui lòng thử lại sau"})
		return
	}
	defer privacySyncMu.Unlock()

	config.ConfigInstance.RwMutx.Lock()
	config.ConfigInstance.EnableGeolocation = geolocation
	config.ConfigInstance.EnableGrove = grove
	config.ConfigInstance.RwMutx.Unlock()
	if err := saveEnvLine("ENABLE_GEOLOCATION", fmt.Sprintf("%t", geolocation)); err != nil {
		logger.Error("Failed to persist ENABLE_GEOLOCATION to .env: %v", err)
	}
	if err := saveEnvLine("ENABLE_GROVE", fmt.Sprintf("%t", grove)); err != nil {
		logger.Error("Failed to persist ENABLE_GROVE to .env: %v", err)
	}

	config.ConfigInstance.RwMutx.RLock()
	sessionsCopy := make([]config.SessionInfo, len(config.ConfigInstance.Sessions))
	copy(sessionsCopy, config.ConfigInstance.Sessions)
	config.ConfigInstance.RwMutx.RUnlock()

	results := make([]syncResult, len(sessionsCopy))
	if len(sessionsCopy) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "Không có tài khoản nào trong pool",
			"results": results,
		})
		return
	}

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
			if err := client.UpdatePrivacySettings(geolocation, grove); err != nil {
				results[idx] = syncResult{Account: label, Status: "error", Message: err.Error()}
				logger.Warn("Privacy sync failed for account %s: %v", label, err)
				return
			}
			gotGeo, gotGrove, err := client.GetAccountPrivacySettings()
			if err != nil {
				results[idx] = syncResult{Account: label, Status: "warning", Message: "đã gửi nhưng không xác minh được: " + err.Error()}
				logger.Warn("Privacy sync verification failed for account %s: %v", label, err)
				return
			}
			msg, verified := privacyVerifyMessage(geolocation, grove, gotGeo, gotGrove)
			if !verified {
				results[idx] = syncResult{Account: label, Status: "warning", Message: msg}
				logger.Warn("Privacy sync warning for account %s: %s", label, msg)
				return
			}
			results[idx] = syncResult{Account: label, Status: "ok", Message: msg}
			logger.Info("Privacy sync succeeded for account %s: %s", label, msg)
		}(i, s)
	}
	wg.Wait()

	c.JSON(http.StatusOK, gin.H{
		"message": "Đã hoàn thành đồng bộ quyền riêng tư",
		"results": results,
	})
}

func privacyVerifyMessage(wantGeo, wantGrove bool, gotGeo, gotGrove *bool) (string, bool) {
	problems := make([]string, 0, 2)
	if wantGeo {
		if gotGeo != nil && !*gotGeo {
			problems = append(problems, "enabled_geolocation vẫn tắt")
		}
	} else if gotGeo == nil || *gotGeo {
		problems = append(problems, "enabled_geolocation vẫn bật/mặc định")
	}
	if gotGrove == nil {
		problems = append(problems, "grove_enabled còn null (có thể chưa accept điều khoản)")
	} else if *gotGrove != wantGrove {
		problems = append(problems, fmt.Sprintf("grove_enabled=%t", *gotGrove))
	}
	if len(problems) > 0 {
		return "đã gửi, chưa xác nhận: " + strings.Join(problems, "; "), false
	}
	return fmt.Sprintf("đã cập nhật (location=%t, improve_models=%t)", wantGeo, wantGrove), true
}

// MemoryGetHandler returns the target value used by the memory sync.
func MemoryGetHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"memory": config.ConfigInstance.MemorySyncSettings()})
}

// MemorySyncHandler applies claude.ai's "Generate memory from chats" setting to
// every account in the pool and records the target value in config and .env.
func MemorySyncHandler(c *gin.Context) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Enabled == nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"enabled\": true|false}"})
		return
	}
	enabled := *body.Enabled

	if !memorySyncMu.TryLock() {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "Đang có thao tác đồng bộ bộ nhớ khác chạy, vui lòng thử lại sau"})
		return
	}
	defer memorySyncMu.Unlock()

	config.ConfigInstance.RwMutx.Lock()
	config.ConfigInstance.EnableMemory = enabled
	config.ConfigInstance.RwMutx.Unlock()
	if err := saveEnvLine("ENABLE_MEMORY", fmt.Sprintf("%t", enabled)); err != nil {
		logger.Error("Failed to persist ENABLE_MEMORY to .env: %v", err)
	}

	config.ConfigInstance.RwMutx.RLock()
	sessionsCopy := make([]config.SessionInfo, len(config.ConfigInstance.Sessions))
	copy(sessionsCopy, config.ConfigInstance.Sessions)
	config.ConfigInstance.RwMutx.RUnlock()

	results := make([]syncResult, len(sessionsCopy))
	if len(sessionsCopy) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "Không có tài khoản nào trong pool",
			"results": results,
		})
		return
	}

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
			if err := client.UpdateMemorySettings(enabled); err != nil {
				results[idx] = syncResult{Account: label, Status: "error", Message: err.Error()}
				logger.Warn("Memory sync failed for account %s: %v", label, err)
				return
			}
			got, err := client.GetAccountMemorySetting()
			// claude.ai applies settings asynchronously, so retry briefly
			// before reporting a warning.
			for attempt := 0; err == nil && !memorySettingMatches(enabled, got) && attempt < 2; attempt++ {
				time.Sleep(time.Second)
				got, err = client.GetAccountMemorySetting()
			}
			if err != nil {
				results[idx] = syncResult{Account: label, Status: "warning", Message: "đã gửi nhưng không xác minh được: " + err.Error()}
				logger.Warn("Memory sync verification failed for account %s: %v", label, err)
				return
			}
			msg, verified := memoryVerifyMessage(enabled, got)
			if !verified {
				results[idx] = syncResult{Account: label, Status: "warning", Message: msg}
				logger.Warn("Memory sync warning for account %s: %s", label, msg)
				return
			}
			results[idx] = syncResult{Account: label, Status: "ok", Message: msg}
			logger.Info("Memory sync succeeded for account %s: %s", label, msg)
		}(i, s)
	}
	wg.Wait()

	c.JSON(http.StatusOK, gin.H{
		"message": "Đã hoàn thành đồng bộ bộ nhớ",
		"results": results,
	})
}

// memorySettingMatches reports whether the read-back value fulfils the target.
// claude.ai stores a null enabled_saffron until the user picks a value and
// treats null as enabled (the upstream default on Free/Pro/Max).
func memorySettingMatches(want bool, got *bool) bool {
	if got == nil {
		return want
	}
	return *got == want
}

func memoryVerifyMessage(want bool, got *bool) (string, bool) {
	if memorySettingMatches(want, got) {
		return fmt.Sprintf("đã cập nhật (memory_from_chats=%t)", want), true
	}
	if got == nil {
		return "đã gửi, chưa xác nhận: enabled_saffron còn null (mặc định bật; có thể chưa áp dụng hoặc bị admin tổ chức chặn)", false
	}
	return fmt.Sprintf("đã gửi, chưa xác nhận: enabled_saffron=%t", *got), false
}

// MCPSyncHandler synchronizes all accounts with a new remote MCP server URL.
func MCPSyncHandler(c *gin.Context) {
	var body struct {
		Name         string           `json:"name"`
		URL          string           `json:"url"`
		AuthMode     string           `json:"authMode"`
		Registration string           `json:"registration"`
		ClientID     string           `json:"clientId"`
		ClientSecret string           `json:"clientSecret"`
		Headers      []core.MCPHeader `json:"headers"`
		// ToolMode / ToolModes are optional: default permission of every tool
		// and per-tool overrides. Omitted fields keep the stored values.
		ToolMode  string            `json:"toolMode"`
		ToolModes map[string]string `json:"toolModes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Dữ liệu JSON không hợp lệ"})
		return
	}
	mcpURL := strings.TrimSpace(body.URL)
	mcpName := strings.TrimSpace(body.Name)
	if mcpName == "" {
		mcpName = "mcp"
	}
	parsedURL, err := url.ParseRequestURI(mcpURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "URL MCP không hợp lệ (phải bắt đầu bằng http:// hoặc https://)"})
		return
	}

	install, err := core.BuildMCPInstall(mcpURL, core.MCPInstallOptions{
		AuthMode:     body.AuthMode,
		Registration: body.Registration,
		ClientID:     body.ClientID,
		ClientSecret: body.ClientSecret,
		Headers:      body.Headers,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}

	toolMode, toolModes, err := resolveMCPPermissions(body.ToolMode, body.ToolModes)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: err.Error()})
		return
	}
	permsGiven := strings.TrimSpace(body.ToolMode) != "" || body.ToolModes != nil

	if !mcpOpMu.TryLock() {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "Đang có thao tác MCP khác chạy, vui lòng thử lại sau"})
		return
	}
	defer mcpOpMu.Unlock()

	if permsGiven {
		saveMCPPermissions(toolMode, toolModes)
	}
	perms := core.MCPToolPermissions{Default: toolMode, Tools: toolModes}

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
			res, syncErr := client.SyncRemoteMCP(mcpName, mcpURL, install, perms)
			if syncErr != nil {
				results[idx] = core.SyncMCPResult{
					Account: label,
					Status:  "error",
					Message: syncErr.Error(),
				}
				logger.Warn("MCP sync failed for account %s: %v", label, syncErr)
			} else {
				res.Account = label
				results[idx] = *res
				logger.Info("MCP sync succeeded for account %s: %s", label, res.Message)
			}
		}(i, s)
	}

	wg.Wait()
	core.ClearMCPCache()

	c.JSON(http.StatusOK, gin.H{
		"message": "Đã hoàn thành đồng bộ MCP",
		"results": results,
	})
}

// MCPDeleteHandler deletes every custom remote MCP server for all accounts in
// the pool. Body: {"key": "<full session key>"} targets a single account;
// an empty body deletes for every account.
func MCPDeleteHandler(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	_ = c.ShouldBindJSON(&body) // empty body = all accounts
	key := strings.TrimSpace(body.Key)

	config.ConfigInstance.RwMutx.RLock()
	sessionsCopy := make([]config.SessionInfo, len(config.ConfigInstance.Sessions))
	copy(sessionsCopy, config.ConfigInstance.Sessions)
	config.ConfigInstance.RwMutx.RUnlock()

	if key != "" {
		filtered := make([]config.SessionInfo, 0, 1)
		for _, s := range sessionsCopy {
			if s.SessionKey == key {
				filtered = append(filtered, s)
			}
		}
		if len(filtered) == 0 {
			c.JSON(http.StatusNotFound, ErrorResponse{Error: "Account not found"})
			return
		}
		sessionsCopy = filtered
	}

	if len(sessionsCopy) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "Không có tài khoản nào trong pool",
			"results": []core.SyncMCPResult{},
		})
		return
	}

	if !mcpOpMu.TryLock() {
		c.JSON(http.StatusConflict, ErrorResponse{Error: "Đang có thao tác MCP khác chạy, vui lòng thử lại sau"})
		return
	}
	defer mcpOpMu.Unlock()

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
			res, delErr := client.DeleteAllRemoteMCP()
			if delErr != nil {
				results[idx] = core.SyncMCPResult{
					Account: label,
					Status:  "error",
					Message: delErr.Error(),
				}
				logger.Warn("MCP delete failed for account %s: %v", label, delErr)
			} else {
				res.Account = label
				results[idx] = *res
				logger.Info("MCP delete for account %s: %s", label, res.Message)
			}
		}(i, s)
	}

	wg.Wait()
	core.ClearMCPCache()

	c.JSON(http.StatusOK, gin.H{
		"message": "Đã hoàn thành xóa MCP custom",
		"results": results,
	})
}

// LastImageGetHandler returns whether only the last message's images are used
func LastImageGetHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"onlyLastImage": config.ConfigInstance.OnlyLastImageEnabled()})
}

// LastImageSetHandler toggles "only last message images" and persists it to .env
func LastImageSetHandler(c *gin.Context) {
	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"enabled\": true|false}"})
		return
	}
	config.ConfigInstance.RwMutx.Lock()
	config.ConfigInstance.OnlyLastImage = body.Enabled
	config.ConfigInstance.RwMutx.Unlock()
	if err := saveEnvLine("ONLY_LAST_IMAGE", fmt.Sprintf("%t", body.Enabled)); err != nil {
		logger.Error("Failed to persist ONLY_LAST_IMAGE to .env: %v", err)
	}
	c.JSON(http.StatusOK, gin.H{"onlyLastImage": body.Enabled})
}

// KeyUnlockHandler manually releases a usage-limit lock before its countdown
// ends (e.g. the quota recovered early). Body: {"key": "<full session key>"}
func KeyUnlockHandler(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Key) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"key\": \"<full session key>\"}"})
		return
	}
	if !config.UnlockLimitSession(strings.TrimSpace(body.Key)) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Account is not limit-locked"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Account unlocked"})
}

// KeyToggleHandler enables/disables an account; disabled accounts are skipped
// by round-robin. Body: {"key": "<full>", "disabled": true|false}
func KeyToggleHandler(c *gin.Context) {
	var body struct {
		Key      string `json:"key"`
		Disabled bool   `json:"disabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Key) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"key\": \"<full session key>\", \"disabled\": true|false}"})
		return
	}
	config.ConfigInstance.RwMutx.Lock()
	found := false
	for i := range config.ConfigInstance.Sessions {
		if config.ConfigInstance.Sessions[i].SessionKey == body.Key {
			config.ConfigInstance.Sessions[i].Disabled = body.Disabled
			found = true
			break
		}
	}
	config.ConfigInstance.RwMutx.Unlock()
	if !found {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Account not found"})
		return
	}
	if err := persistCookieAccounts(); err != nil {
		logger.Error("Failed to persist accounts: %v", err)
	}
	state := "enabled"
	if body.Disabled {
		state = "disabled"
	}
	c.JSON(http.StatusOK, gin.H{"message": "Account " + state})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// persistCookieAccounts serializes the current account pool (the cookie array
// + display name of each session) back to accounts.json. Sessions without a
// cookie array (e.g. env fallback) are skipped.
func persistCookieAccounts() error {
	config.ConfigInstance.RwMutx.RLock()
	accounts := make([]config.CookieAccount, 0, len(config.ConfigInstance.Sessions))
	for i := range config.ConfigInstance.Sessions {
		s := &config.ConfigInstance.Sessions[i]
		if len(s.CookieJSON) == 0 {
			continue
		}
		accounts = append(accounts, config.CookieAccount{
			Name:     s.DisplayName,
			Disabled: s.Disabled,
			Cookies:  s.CookieJSON,
		})
	}
	config.ConfigInstance.RwMutx.RUnlock()

	if len(accounts) == 0 {
		return nil
	}
	data, err := json.MarshalIndent(accounts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("accounts.json", data, 0o600)
}

// resolveAccountEmail builds a browser-fingerprinted client for the session's
// cookie and asks claude.ai for the account email. Returns "" on any failure;
// callers use it only as a best-effort display label.
func resolveAccountEmail(s *config.SessionInfo) string {
	client := core.NewClientWithCookie(s.SessionKey, config.ConfigInstance.Proxy, "", s.ExtraCookie, s.DeviceID)
	email, err := client.GetAccountEmail()
	if err != nil {
		logger.Error("Failed to resolve account email: %v", err)
		return ""
	}
	return email
}

// saveEnvLine rewrites a single KEY=value line in .env (creates the file if missing).
// Duplicate lines of the same key collapse into one; an empty value keeps the
// line as "KEY=" so it can be filled in later.
func saveEnvLine(name, value string) error {
	envFileMu.Lock()
	defer envFileMu.Unlock()

	path := ".env"
	newLine := name + "=" + value
	lines := []string{}
	found := false
	if f, err := os.Open(path); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			text := scanner.Text()
			if strings.HasPrefix(text, name+"=") {
				if !found {
					lines = append(lines, newLine)
					found = true
				}
				// drop duplicate lines of the same key
				continue
			}
			lines = append(lines, text)
		}
		f.Close()
	} else if !os.IsNotExist(err) {
		return err
	}
	if !found {
		lines = append(lines, newLine)
	}

	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return nil
}
