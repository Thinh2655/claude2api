package service

import (
	"claude2api/config"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMCPSyncSlotOptIn(t *testing.T) {
	// An empty pool exercises request validation without contacting Claude.
	previous := config.ConfigInstance
	config.ConfigInstance = &config.Config{}
	t.Cleanup(func() { config.ConfigInstance = previous })

	router := gin.New()
	router.POST("/mcp/sync", MCPSyncHandler)
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"legacy SSE URL", `{"url":"https://example.com/sse"}`, http.StatusOK},
		{"slots disabled", `{"url":"https://example.com/custom","perAccountSlots":false}`, http.StatusOK},
		{"slots enabled", `{"url":"https://example.com/mcp/","perAccountSlots":true}`, http.StatusOK},
		{"slots require a base URL", `{"url":"https://example.com/mcp/s1","perAccountSlots":true}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/mcp/sync", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("got status %d, want %d: %s", response.Code, tc.want, response.Body.String())
			}
		})
	}
}
