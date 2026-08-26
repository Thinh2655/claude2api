package service

import (
	"bufio"
	"claude2api/config"
	"claude2api/logger"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

type keyInfo struct {
	Masked   string `json:"masked"`
	Full     string `json:"full,omitempty"`
	HasOrgID bool   `json:"hasOrgID"`
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

// KeysListHandler returns all configured sessions with masked keys
func KeysListHandler(c *gin.Context) {
	config.ConfigInstance.RwMutx.RLock()
	defer config.ConfigInstance.RwMutx.RUnlock()

	resp := keysResponse{Keys: make([]keyInfo, 0, len(config.ConfigInstance.Sessions))}
	for i := range config.ConfigInstance.Sessions {
		s := &config.ConfigInstance.Sessions[i]
		resp.Keys = append(resp.Keys, keyInfo{
			Masked:   maskKey(s.SessionKey),
			Full:     s.SessionKey,
			HasOrgID: s.OrgID != "",
		})
	}
	c.JSON(http.StatusOK, resp)
}

// KeyAddHandler adds one or more new session keys (comma-separated in body)
func KeyAddHandler(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Key) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"key\": \"sk-ant-sid02-...\"}"})
		return
	}

	added := 0
	for _, part := range strings.Split(body.Key, ",") {
		key := strings.TrimSpace(strings.Trim(part, `"`))
		if key == "" {
			continue
		}
		if !strings.HasPrefix(key, "sk-ant-sid") {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error: fmt.Sprintf("Invalid session key format: %s...", key[:min(20, len(key))])})
			return
		}
		if config.ConfigInstance.AddSession(key) {
			added++
		}
	}
	if added == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "No new keys added (duplicate)"})
		return
	}
	if err := saveSessionsToEnv(); err != nil {
		logger.Error(fmt.Sprintf("Failed to persist sessions to .env: %v", err))
	}
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Added %d key(s)", added)})
}

// KeyDeleteHandler removes a session by full key
func KeyDeleteHandler(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Key) == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Provide {\"key\": \"<full session key>\"}"})
		return
	}
	if len(config.ConfigInstance.Sessions) <= 1 {
		c.JSON(http.StatusBadRequest, ErrorResponse{Error: "Cannot delete the last remaining key"})
		return
	}
	if !config.ConfigInstance.RemoveSession(strings.TrimSpace(body.Key)) {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Key not found"})
		return
	}
	if err := saveSessionsToEnv(); err != nil {
		logger.Error(fmt.Sprintf("Failed to persist sessions to .env: %v", err))
	}
	c.JSON(http.StatusOK, gin.H{"message": "Key deleted"})
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// saveSessionsToEnv rewrites only the SESSIONS line in .env, preserving other lines.
func saveSessionsToEnv() error {
	config.ConfigInstance.RwMutx.RLock()
	keys := make([]string, 0, len(config.ConfigInstance.Sessions))
	for i := range config.ConfigInstance.Sessions {
		k := config.ConfigInstance.Sessions[i].SessionKey
		line := k
		if org := config.ConfigInstance.Sessions[i].OrgID; org != "" {
			line = k + ":" + org
		}
		keys = append(keys, line)
	}
	config.ConfigInstance.RwMutx.RUnlock()

	return saveEnvLine("SESSIONS", strings.Join(keys, ","))
}

// saveEnvLine rewrites a single KEY=value line in .env (creates the file if missing).
// An empty value keeps the line as "KEY=" so it can be filled in later.
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
				lines = append(lines, newLine)
				found = true
			} else {
				lines = append(lines, text)
			}
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
