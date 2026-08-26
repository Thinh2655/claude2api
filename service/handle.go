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
	models := []map[string]interface{}{
		{"id": "claude-sonnet-4-6"},
		{"id": "claude-haiku-4-5-20251001"},
		{"id": "claude-sonnet-5"},
	}

	extendedModels := make([]map[string]interface{}, 0, len(models)*2)
	for _, m := range models {
		// 保留原有 id
		extendedModels = append(extendedModels, m)
		// 追加 -think 版本
		if id, ok := m["id"].(string); ok {
			extendedModels = append(extendedModels, map[string]interface{}{
				"id": id + "-think",
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": extendedModels,
	})
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
	index := config.Sr.NextIndex()
	// Attempt with retry mechanism - no local cooldown bookkeeping: a 429
	// just moves to the next session immediately
	for i := 0; i < config.ConfigInstance.RetryCount; i++ {
		index = (index + 1) % len(config.ConfigInstance.Sessions)
		session, err := config.ConfigInstance.GetSessionForModel(index)
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get session for model %s: %v", model, err))
			logger.Info("Retrying another session")
			continue
		}

		logger.Info(fmt.Sprintf("Using session for model %s: %s", model, session.SessionKey))
		// Initialize client and process request
		if handleChatRequest(c, session, model, processor, req.Stream) {
			return // Success, exit the retry loop
		}

		// Client disconnected - no point in retrying
		select {
		case <-c.Request.Context().Done():
			logger.Info("Client closed connection, stop retrying")
			return
		default:
		}

		// If we're here, the request failed - retry with another session
		logger.Info("Retrying another session")
	}

	logger.Error("Failed for all retries")
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: "Failed to process request after multiple attempts"})
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

	// Extract session info from auth header
	session, err := extractSessionFromAuthHeader(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: fmt.Sprintf("Invalid authorization: %v", err),
		})
		return
	}

	// Process the request with the provided session
	if !handleChatRequest(c, session, model, processor, req.Stream) {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: "Failed to process request",
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

	return &req, nil
}

func getModelOrDefault(model string) string {
	if model == "" {
		return "claude-sonnet-4-6"
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

func handleChatRequest(c *gin.Context, session *config.SessionInfo, model string, processor *utils.ChatRequestProcessor, stream bool) bool {
	session.Lock()
	defer session.Unlock()

	// Initialize the Claude client
	claudeClient := core.NewClient(session.SessionKey, config.ConfigInstance.Proxy, model)

	// Get org ID if not already set
	if session.OrgID == "" {
		orgId, err := claudeClient.GetOrgID()
		if err != nil {
			logger.Error(fmt.Sprintf("Failed to get org ID: %v", err))
			return false
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
			return false
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
		return false
	}
	if statusCode != http.StatusOK {
		return false
	}

	// Clean up conversation if enabled
	if config.ConfigInstance.ChatDelete {
		go cleanupConversation(claudeClient, conversationID, 3)
	}

	return true
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
