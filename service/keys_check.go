package service

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"

	"claude2api/config"
	"claude2api/core"
)

// KeyCheckResult is the outcome of validating a single session key
type KeyCheckResult struct {
	Key     string `json:"key"`
	Status  string `json:"status"` // valid | rate_limited | invalid | error
	Detail  string `json:"detail,omitempty"`
}

// KeyCheckHandler validates one or all session keys against claude.ai.
// Body: {"key": "<optional full key>"} — omit to check every key.
func KeyCheckHandler(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	_ = c.ShouldBindJSON(&body)

	config.ConfigInstance.RwMutx.RLock()
	type target struct {
		key string
		idx int
	}
	var targets []target
	if body.Key != "" {
		for i := range config.ConfigInstance.Sessions {
			if config.ConfigInstance.Sessions[i].SessionKey == body.Key {
				targets = append(targets, target{key: body.Key, idx: i})
				break
			}
		}
	} else {
		for i := range config.ConfigInstance.Sessions {
			targets = append(targets, target{key: config.ConfigInstance.Sessions[i].SessionKey, idx: i})
		}
	}
	config.ConfigInstance.RwMutx.RUnlock()

	if len(targets) == 0 {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Key not found in pool"})
		return
	}

	results := make([]KeyCheckResult, 0, len(targets))
	client := core.GatewayClient("", 20*time.Second)
	for _, t := range targets {
		results = append(results, checkOneKey(client, t.key))
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

func checkOneKey(client *req.Client, key string) KeyCheckResult {
	client.SetCommonCookies(&http.Cookie{Name: "sessionKey", Value: key})
	resp, err := client.R().
		SetHeader("referer", "https://claude.ai/new").
		SetHeader("accept", "application/json").
		Get("https://claude.ai/api/organizations")
	if err != nil {
		return KeyCheckResult{Key: key, Status: "error", Detail: err.Error()}
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return KeyCheckResult{Key: key, Status: "valid"}
	case http.StatusUnauthorized, http.StatusForbidden:
		return KeyCheckResult{Key: key, Status: "invalid", Detail: fmt.Sprintf("claude.ai returned %d (key expired or revoked)", resp.StatusCode)}
	case http.StatusTooManyRequests:
		return KeyCheckResult{Key: key, Status: "rate_limited", Detail: "claude.ai returned 429 (quota exhausted, resets in ~2 min)"}
	default:
		return KeyCheckResult{Key: key, Status: "error", Detail: fmt.Sprintf("claude.ai returned %d", resp.StatusCode)}
	}
}
