package service

import (
	"bufio"
	"claude2api/config"
	"claude2api/core"
	"claude2api/logger"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

type keyInfo struct {
	Masked      string `json:"masked"`
	Full        string `json:"full,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	HasCookie   bool   `json:"hasCookie"`
	HasOrgID    bool   `json:"hasOrgID"`
	Disabled    bool   `json:"disabled"`
}

type keysResponse struct {
	Keys []keyInfo `json:"keys"`
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
// display label (account name if set, otherwise a masked key)
func KeysListHandler(c *gin.Context) {
	config.ConfigInstance.RwMutx.RLock()
	defer config.ConfigInstance.RwMutx.RUnlock()

	resp := keysResponse{Keys: make([]keyInfo, 0, len(config.ConfigInstance.Sessions))}
	for i := range config.ConfigInstance.Sessions {
		s := &config.ConfigInstance.Sessions[i]
		resp.Keys = append(resp.Keys, keyInfo{
			Masked:      maskKey(s.SessionKey),
			Full:        s.SessionKey,
			DisplayName: s.DisplayName,
			HasCookie:   s.ExtraCookie != "",
			HasOrgID:    s.OrgID != "",
			Disabled:    s.Disabled,
		})
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
		logger.Error(fmt.Sprintf("Failed to persist accounts: %v", err))
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
		logger.Error(fmt.Sprintf("Failed to persist accounts: %v", err))
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
		logger.Error(fmt.Sprintf("Failed to persist GATEWAY_KEY to .env: %v", err))
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
		logger.Error(fmt.Sprintf("Failed to persist ENABLE_TOOLS to .env: %v", err))
	}
	c.JSON(http.StatusOK, gin.H{"enableTools": body.Enabled})
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
		logger.Error(fmt.Sprintf("Failed to persist accounts: %v", err))
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
		logger.Error(fmt.Sprintf("Failed to resolve account email: %v", err))
		return ""
	}
	return email
}

// saveEnvLine rewrites a single KEY=value line in .env (creates the file if missing).
// Duplicate lines of the same key collapse into one; an empty value keeps the
// line as "KEY=" so it can be filled in later.
func saveEnvLine(name, value string) error {
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

	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(out)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	if err := w.Flush(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
