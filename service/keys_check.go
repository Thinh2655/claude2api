package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/imroc/req/v3"

	"claude2api/config"
)

// KeyCheckResult is the outcome of validating a single account against claude.ai.
type KeyCheckResult struct {
	Key    string `json:"key"`
	Label  string `json:"label,omitempty"`
	Status string `json:"status"` // valid | rate_limited | invalid | error
	Detail string `json:"detail,omitempty"`
}

// KeyCheckHandler validates one or all accounts against claude.ai.
// Body: {"key": "<optional full key>"} — omit to check every account.
func KeyCheckHandler(c *gin.Context) {
	var body struct {
		Key string `json:"key"`
	}
	_ = c.ShouldBindJSON(&body)

	config.ConfigInstance.RwMutx.RLock()
	type target struct {
		session config.SessionInfo
	}
	var targets []target
	if body.Key != "" {
		for i := range config.ConfigInstance.Sessions {
			if config.ConfigInstance.Sessions[i].SessionKey == body.Key {
				targets = append(targets, target{session: config.ConfigInstance.Sessions[i]})
				break
			}
		}
	} else {
		for i := range config.ConfigInstance.Sessions {
			targets = append(targets, target{session: config.ConfigInstance.Sessions[i]})
		}
	}
	config.ConfigInstance.RwMutx.RUnlock()

	if len(targets) == 0 {
		c.JSON(http.StatusNotFound, ErrorResponse{Error: "Account not found in pool"})
		return
	}

	results := make([]KeyCheckResult, 0, len(targets))
	for _, t := range targets {
		results = append(results, checkOneAccount(t.session))
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

// checkOneAccount probes claude.ai with the account's full browser cookie to
// classify it as valid / rate_limited / invalid / error.
func checkOneAccount(s config.SessionInfo) KeyCheckResult {
	label := s.DisplayName
	if label == "" {
		label = maskKey(s.SessionKey)
	}
	client := req.C().ImpersonateChrome().SetTimeout(20 * time.Second)
	if config.ConfigInstance.Proxy != "" {
		client.SetProxyURL(config.ConfigInstance.Proxy)
	}
	if s.DeviceID != "" {
		client.SetCommonHeader("anthropic-device-id", s.DeviceID)
	}
	cookie := s.ExtraCookie
	if cookie == "" {
		// Env-only session: send just the sessionKey.
		cookie = "sessionKey=" + s.SessionKey
	}
	client.SetCommonHeader("Cookie", cookie)
	resp, err := client.R().
		SetHeader("referer", "https://claude.ai/new").
		SetHeader("accept", "application/json").
		Get("https://claude.ai/api/organizations")
	if err != nil {
		return KeyCheckResult{Key: s.SessionKey, Label: label, Status: "error", Detail: err.Error()}
	}
	switch resp.StatusCode {
	case http.StatusOK:
		// Account is reachable: fetch its email to use as the display name,
		// persisting it so the UI shows a label without a follow-up request.
		if email := fetchAccountEmail(client); email != "" {
			if s.DisplayName == "" {
				config.ConfigInstance.SetSessionDisplayName(s.SessionKey, email)
				_ = persistCookieAccounts()
				label = email
			}
		}
		return KeyCheckResult{Key: s.SessionKey, Label: label, Status: "valid"}
	case http.StatusUnauthorized, http.StatusForbidden:
		return KeyCheckResult{Key: s.SessionKey, Label: label, Status: "invalid", Detail: fmt.Sprintf("claude.ai returned %d (cookie expired or revoked)", resp.StatusCode)}
	case http.StatusTooManyRequests:
		return KeyCheckResult{Key: s.SessionKey, Label: label, Status: "rate_limited", Detail: "claude.ai returned 429 (quota exhausted, resets in ~2 min)"}
	default:
		return KeyCheckResult{Key: s.SessionKey, Label: label, Status: "error", Detail: fmt.Sprintf("claude.ai returned %d", resp.StatusCode)}
	}
}

// fetchAccountEmail calls /api/account with the same client/cookie and returns
// the account's email, or "" on any failure.
func fetchAccountEmail(client *req.Client) string {
	resp, err := client.R().
		SetHeader("referer", "https://claude.ai/").
		SetHeader("accept", "application/json").
		Get("https://claude.ai/api/account")
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}
	var acct struct {
		EmailAddress string `json:"email_address"`
	}
	if err := json.Unmarshal(resp.Bytes(), &acct); err != nil {
		return ""
	}
	return acct.EmailAddress
}
