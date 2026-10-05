package service

import (
	"claude2api/config"
	"claude2api/core"
	"claude2api/logger"
	"claude2api/model"
	"claude2api/utils"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

// HealthCheckHandler handles the health check endpoint
func HealthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
	})
}

func MoudlesHandler(c *gin.Context) {
	// Prefer the live model list fetched from claude.ai; fall back to the
	// built-in table when no session has been probed yet.
	if ids, ok := core.BootstrapModelIDs(); ok {
		data := make([]map[string]interface{}, 0, len(ids))
		for _, id := range ids {
			data = append(data, map[string]interface{}{"id": id})
		}
		c.JSON(http.StatusOK, gin.H{"data": data})
		return
	}
	ids := core.ModelIDs()
	data := make([]map[string]interface{}, 0, len(ids))
	for _, id := range ids {
		data = append(data, map[string]interface{}{"id": id})
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// claude.ai retired the old model IDs; requests with them fail with 400
// "Unsupported model". Map them onto the IDs claude.ai currently accepts so
// existing API clients keep working.
var legacyModelAliases = map[string]string{
	"claude-3-7-sonnet-20250219": "claude-sonnet-4-6",
	"claude-sonnet-4-20250514":   "claude-sonnet-4-6",
	"claude-sonnet-4-5-20250929": "claude-sonnet-4-6",
	"claude-opus-4-20250514":     "claude-sonnet-4-6",
	"claude-opus-4-1-20250805":   "claude-sonnet-4-6",
}

func resolveModel(model string) string {
	think := strings.HasSuffix(model, "-think")
	base := strings.TrimSuffix(model, "-think")
	if resolved, ok := legacyModelAliases[base]; ok {
		base = resolved
	}
	if think {
		return base + "-think"
	}
	return base
}

// ChatCompletionsHandler handles the chat completions endpoint
func ChatCompletionsHandler(c *gin.Context) {
	useMirror, exist := c.Get("UseMirrorApi")
	if exist && useMirror.(bool) {
		MirrorChatHandler(c)
		return
	}

	// Parse and validate request
	req, err := parseAndValidateRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Process messages into prompt and extract images
	processor := utils.NewChatRequestProcessor()
	processor.ProcessMessages(req.Messages)

	// Get model or use default
	model := getModelOrDefault(req.Model)
	// Echo the resolved model id back in the response instead of a hardcoded
	// one, so OpenAI clients see the model they asked for.
	c.Set("RequestedModel", model)
	// Rough input size, so the OpenAI usage object and the dashboard are not
	// all-zero: claude.ai never reports token counts.
	c.Set("PromptTokens", utils.EstimatePromptTokens(processor.Prompt.String()))
	// Round-robin over EVERY account: a failed attempt parks that account for
	// sessionCooldownTTL and the request moves on immediately, so all sessions
	// are tried before the request is reported as failed.
	// Remember the last upstream status so that if every session fails the
	// underlying cause (e.g. a rate limit) is reported to the client instead
	// of a generic 500.
	order := roundRobinOrder(config.ConfigInstance.Sessions, config.Sr.NextIndex())

	var lastStatus int
	var lastErrMsg string
	for _, index := range order {
		session, err := config.ConfigInstance.GetSessionForModel(index)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get session for model %s: %v", model, err))
			lastStatus = http.StatusServiceUnavailable
			continue
		}

		label := session.DisplayName
		if label == "" {
			label = maskKey(session.SessionKey)
		}
		logger.Info(fmt.Sprintf("Using account for model %s: %s", model, label))
		// Initialize client and process request
		st, ok, errMsg := handleChatRequest(c, session, model, processor, req.Stream)
		if ok {
			return // Success, exit the retry loop
		}
		if st != 0 {
			lastStatus = st
		}
		if errMsg != "" {
			lastErrMsg = errMsg
		}

		// Client disconnected - no point in retrying
		select {
		case <-c.Request.Context().Done():
			logger.Info("Client closed connection, stop retrying")
			return
		default:
		}

		// Usage-limit hit ("You've hit your limit for Claude messages", 429,
		// session/weekly reset): lock the account until reset instead of the
		// short 30s cooldown, so later requests skip it entirely and don't
		// waste an upstream call that is guaranteed to fail.
		if isLimitFailure(st, errMsg) {
			ttl := core.ParseLimitReset(errMsg)
			if ttl <= 0 {
				ttl = config.DefaultLimitTTL
				logger.Info(fmt.Sprintf("Limit message has no parseable reset time, fallback %s: %s", ttl, core.LimitReasonSnippet(errMsg)))
			}
			reason := core.LimitReasonSnippet(errMsg)
			if reason == "" {
				reason = http.StatusText(st)
			}
			until := config.LockLimitSession(session.SessionKey, reason, ttl)
			logger.Info(fmt.Sprintf("Account %s hit usage limit, locked until %s (%s)", label, until.Format("15:04:05"), reason))
		} else {
			// Park the failing account so later requests skip it for a while
			// instead of wasting time on a session that just rate-limited.
			config.CooldownSession(session.SessionKey)
		}
		logger.Info("Retrying another session")
	}

	if len(order) == 0 {
		if rem, until, ok := minLimitWait(); ok {
			c.JSON(http.StatusTooManyRequests, ErrorResponse{
				Error: fmt.Sprintf("All accounts are temporarily limited (earliest reset in %s at %s)", formatWait(rem), until.Format("15:04:05")),
			})
			return
		}
		lastStatus = http.StatusServiceUnavailable
	}
	logger.Error("Failed for all retries")
	// A limit failure that surfaced as SSE error (status 500 + limit text) is
	// still a quota problem for the client: report 429 so callers back off
	// instead of treating it as a generic server error.
	if isLimitFailure(lastStatus, lastErrMsg) {
		msg := "All accounts hit usage limit"
		if lastErrMsg != "" {
			msg += ": " + core.LimitReasonSnippet(lastErrMsg)
		} else if rem, until, ok := minLimitWait(); ok {
			msg = fmt.Sprintf("All accounts are temporarily limited (earliest reset in %s at %s)", formatWait(rem), until.Format("15:04:05"))
		}
		c.JSON(http.StatusTooManyRequests, ErrorResponse{Error: msg})
		return
	}
	finalStatus := http.StatusInternalServerError
	finalMsg := "Failed to process request after multiple attempts"
	if lastStatus != 0 && lastStatus != http.StatusOK {
		finalStatus = lastStatus
		finalMsg = "Upstream claude.ai is unavailable: " + http.StatusText(lastStatus)
	}
	c.JSON(finalStatus, ErrorResponse{Error: finalMsg})
}

func MirrorChatHandler(c *gin.Context) {
	if !config.ConfigInstance.EnableMirrorApi {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error: "Mirror API is not enabled",
		})
		return
	}

	// Parse and validate request
	req, err := parseAndValidateRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Process messages into prompt and extract images
	processor := utils.NewChatRequestProcessor()
	processor.ProcessMessages(req.Messages)

	// Get model or use default
	model := getModelOrDefault(req.Model)

	// Echo the resolved model id back in the response instead of a hardcoded
	// one, so OpenAI clients see the model they asked for.
	c.Set("RequestedModel", model)
	// Rough input size for the usage object and dashboard (upstream reports none).
	c.Set("PromptTokens", utils.EstimatePromptTokens(processor.Prompt.String()))

	// Extract session info from auth header
	session, err := extractSessionFromAuthHeader(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid authorization: %v", err),
		})
		return
	}

	// Process the request with the provided session
	if status, ok, errMsg := handleChatRequest(c, session, model, processor, req.Stream); !ok {
		if isLimitFailure(status, errMsg) {
			ttl := core.ParseLimitReset(errMsg)
			if ttl <= 0 {
				ttl = config.DefaultLimitTTL
			}
			reason := core.LimitReasonSnippet(errMsg)
			if reason == "" {
				reason = http.StatusText(status)
			}
			// Mirror uses an explicit session (not the pool), but the lock is
			// still keyed by sessionKey so the dashboard countdown shows it.
			if session.SessionKey != "" {
				config.LockLimitSession(session.SessionKey, reason, ttl)
			}
			msg := "Upstream claude.ai usage limit: " + reason
			c.JSON(http.StatusTooManyRequests, ErrorResponse{Error: msg})
			return
		}
		finalStatus := status
		finalMsg := "Failed to process request"
		if finalStatus != 0 && finalStatus != http.StatusOK {
			finalStatus = status
			finalMsg = "Upstream claude.ai is unavailable: " + http.StatusText(status)
		}
		c.JSON(finalStatus, ErrorResponse{
			Error: finalMsg,
		})
		return
	}
}

// Helper functions

func parseAndValidateRequest(c *gin.Context) (*model.ChatCompletionRequest, error) {
	var req model.ChatCompletionRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid request: %v", err),
		})
		return nil, err
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: "No messages provided",
		})
		return nil, fmt.Errorf("no messages provided")
	}

	// Stash the client-requested tools so handleChatRequest can forward them
	// upstream (stored bare to avoid the model-param shadowing the model pkg).
	c.Set("ChatTools", req.Tools)

	return &req, nil
}

func getModelOrDefault(model string) string {
	if model == "" {
		return "claude-sonnet-5"
	}
	return resolveModel(model)
}

func extractSessionFromAuthHeader(c *gin.Context) (*config.SessionInfo, error) {
	authInfo := c.Request.Header.Get("Authorization")
	authInfo = strings.TrimPrefix(authInfo, "Bearer ")

	if authInfo == "" {
		return config.NewSessionInfo("", ""), fmt.Errorf("missing authorization header")
	}

	if strings.Contains(authInfo, ":") {
		parts := strings.Split(authInfo, ":")
		return config.NewSessionInfo(parts[0], parts[1]), nil
	}

	return config.NewSessionInfo(authInfo, ""), nil
}

// roundRobinOrder lists every usable session index once starting at start,
// with accounts still in cooldown pushed to the end (so a request that needs a
// fallback tries a cooling account only when no fresh one remains). Manually
// disabled accounts and usage-limit-locked accounts are skipped entirely, so a
// request never wastes an upstream call on an account that is guaranteed to
// answer "You've hit your limit".
func roundRobinOrder(sessions []config.SessionInfo, start int) []int {
	n := len(sessions)
	if n == 0 {
		return nil
	}
	start %= n
	var fresh, cooling []int
	for i := 0; i < n; i++ {
		idx := (start + i) % n
		if sessions[idx].Disabled {
			continue
		}
		if _, _, _, limited := config.SessionLimited(sessions[idx].SessionKey); limited {
			continue
		}
		if config.SessionCooling(sessions[idx].SessionKey) {
			cooling = append(cooling, idx)
		} else {
			fresh = append(fresh, idx)
		}
	}
	return append(fresh, cooling...)
}

// isLimitFailure reports whether a failed upstream attempt means the account
// hit its Claude message quota: HTTP 429, or any error text matching the
// "You've hit your limit" family (completion 4xx with limit body, or an SSE
// error event before any content was streamed).
func isLimitFailure(status int, errMsg string) bool {
	if status == http.StatusTooManyRequests {
		return true
	}
	return errMsg != "" && core.IsUsageLimitMessage(errMsg)
}

// minLimitWait returns the shortest remaining lock among limit-locked pool
// accounts, for "retry after" messages.
func minLimitWait() (rem time.Duration, until time.Time, ok bool) {
	config.ConfigInstance.RwMutx.RLock()
	keys := make([]string, 0, len(config.ConfigInstance.Sessions))
	for i := range config.ConfigInstance.Sessions {
		keys = append(keys, config.ConfigInstance.Sessions[i].SessionKey)
	}
	config.ConfigInstance.RwMutx.RUnlock()
	first := true
	for _, k := range keys {
		u, r, _, limited := config.SessionLimited(k)
		if !limited {
			continue
		}
		if first || r < rem {
			rem, until, ok, first = r, u, true, false
		}
	}
	return rem, until, ok
}

// formatWait renders a countdown duration as H:MM:SS or M:SS for messages.
func formatWait(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int(d.Seconds())
	h, m, sec := s/3600, (s%3600)/60, s%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

func handleChatRequest(c *gin.Context, session *config.SessionInfo, model string, processor *utils.ChatRequestProcessor, stream bool) (status int, ok bool, errMsg string) {
	start := time.Now()
	session.Lock()
	defer session.Unlock()

	// Record this attempt in the dashboard's recent-request log no matter which
	// path the function takes, so the list reflects failures too.
	defer func() {
		recordRecent(c, session, model, status, ok, time.Since(start), firstUserText(processor))
	}()

	fail := func(status int, msg string) (int, bool, string) {
		return status, false, msg
	}

	// Function-calling shim: the web backend has no tools API, so client tool
	// results come back as role=tool text and client function tools are
	// described in the prompt (the model answers with a ```toolcall block).
	if config.ConfigInstance.ToolsEnabled() {
		if v, ok := c.Get("ChatTools"); ok {
			if tools, ok := v.([]map[string]interface{}); ok && len(tools) > 0 {
				if p := core.ToolPrompt(tools); p != "" {
					processor.Prompt.WriteString(p)
				}
			}
		}
	}

	// Initialize the Claude client
	claudeClient := core.NewClientWithCookie(session.SessionKey, config.ConfigInstance.Proxy, model, session.ExtraCookie, session.DeviceID)
	// Forward client-requested tools (web_search, artifacts, repl/code_interpreter)
	// to the upstream tool list; unknown function tools are dropped since the
	// web backend can't execute arbitrary functions.
	if config.ConfigInstance.ToolsEnabled() {
		if v, ok := c.Get("ChatTools"); ok {
			if tools, ok := v.([]map[string]interface{}); ok {
				claudeClient.SetTools(tools)
			}
		}
	}

	// Get org ID if not already set
	if session.OrgID == "" {
		orgId, err := claudeClient.GetOrgID()
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get org ID: %v", err))
			return fail(http.StatusServiceUnavailable, err.Error())
		}
		session.OrgID = orgId
		config.ConfigInstance.SetSessionOrgID(session.SessionKey, session.OrgID)
	}

	claudeClient.SetOrgID(session.OrgID)

	// Upload images if any
	if len(processor.ImgDataList) > 0 {
		err := claudeClient.UploadFile(processor.ImgDataList)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to upload file: %v", err))
			return fail(http.StatusServiceUnavailable, err.Error())
		}
	}

	// Handle large context if needed
	if processor.Prompt.Len() > config.ConfigInstance.MaxChatHistoryLength {
		claudeClient.SetBigContext(processor.Prompt.String())
		processor.ResetForBigContext()
		logger.Info(fmt.Sprintf("Prompt length exceeds max limit (%d), using file context", config.ConfigInstance.MaxChatHistoryLength))
	}

	// Single-request flow: create_conversation_params embedded in completion,
	// mirroring the browser payload (halves upstream requests per message)
	conversationID := claudeClient.PeekNewConversationID()
	_, statusCode, err := claudeClient.SendMessageWithCreate(processor.Prompt.String(), stream, c)
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to send message: %v", err))
		return fail(statusCode, err.Error())
	}
	if statusCode != http.StatusOK {
		return fail(statusCode, "")
	}

	// Clean up conversation if enabled
	if config.ConfigInstance.ChatDelete {
		go cleanupConversation(claudeClient, conversationID, 3)
	}

	return statusCode, true, ""
}

func cleanupConversation(client *core.Client, conversationID string, retry int) {
	for i := 0; i < retry; i++ {
		if err := client.DeleteConversation(conversationID); err != nil {
			logger.Error(fmt.Sprintf("Failed to delete conversation: %v", err))
			time.Sleep(2 * time.Second)
			continue
		}
		logger.Info(fmt.Sprintf("Successfully deleted conversation: %s", conversationID))
		return // 成功后直接返回，不执行后面的错误日志
	}
	// 只有当所有重试都失败后，才会执行到这里
	logger.Error(fmt.Sprintf("Cleanup %s conversation %s failed after %d retries", client.SessionKey, conversationID, retry))
}
