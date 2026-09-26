package service

import (
	"claude2api/config"
	"claude2api/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// recentRequest is one entry of the last-N request log shown in the dashboard.
type recentRequest struct {
	Time             time.Time `json:"time"`
	Account          string    `json:"account"`
	Model            string    `json:"model"`
	Status           int       `json:"status"`
	OK               bool      `json:"ok"`
	Ms               int64     `json:"ms"`
	PromptTokens     int       `json:"promptTokens"`
	CompletionTokens int       `json:"completionTokens"`
	Preview          string    `json:"preview"`
}

const recentLimit = 5

var (
	recentMu      sync.Mutex
	recentRecords [recentLimit]recentRequest
	recentCount   int
	recentNext    int
)

// recordRecent appends a finished request to the ring buffer. Called from every
// return path of handleChatRequest via defer, so no request is missed.
func recordRecent(c *gin.Context, session *config.SessionInfo, model string, status int, ok bool, d time.Duration, prompt string) {
	preview := strings.ReplaceAll(prompt, "\n", " ")
	if len(preview) > 80 {
		preview = preview[:80] + "…"
	}
	promptTokens, _ := c.Get("PromptTokens")
	completionTokens, _ := c.Get("CompletionTokens")
	pt, _ := promptTokens.(int)
	ct, _ := completionTokens.(int)
	rec := recentRequest{
		Time:             time.Now(),
		Account:          displayLabel(session),
		Model:            model,
		Status:           status,
		OK:               ok,
		Ms:               d.Milliseconds(),
		PromptTokens:     pt,
		CompletionTokens: ct,
		Preview:          strings.TrimSpace(preview),
	}
	recentMu.Lock()
	recentRecords[recentNext] = rec
	recentNext = (recentNext + 1) % recentLimit
	if recentCount < recentLimit {
		recentCount++
	}
	recentMu.Unlock()
	broadcastRecent()
}

// recentSnapshot returns the stored requests newest-first.
func recentSnapshot() []recentRequest {
	recentMu.Lock()
	out := make([]recentRequest, 0, recentCount)
	for i := 0; i < recentCount; i++ {
		out = append(out, recentRecords[(recentNext-1-i+recentLimit)%recentLimit])
	}
	recentMu.Unlock()
	return out
}

// RecentRequestsHandler serves the last few requests for the dashboard.
func RecentRequestsHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"requests": recentSnapshot()})
}

// recentSubs are the open dashboard streams waiting for a push. Each channel
// carries one pending notification; sends never block the request path.
var (
	recentSubMu sync.Mutex
	recentSubs  = map[chan struct{}]struct{}{}
)

func broadcastRecent() {
	recentSubMu.Lock()
	defer recentSubMu.Unlock()
	for ch := range recentSubs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// RecentStreamHandler pushes the recent-request list over SSE every time a
// chat request finishes, so the dashboard updates without polling.
func RecentStreamHandler(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.Flush()

	ch := make(chan struct{}, 1)
	recentSubMu.Lock()
	recentSubs[ch] = struct{}{}
	recentSubMu.Unlock()
	defer func() {
		recentSubMu.Lock()
		delete(recentSubs, ch)
		recentSubMu.Unlock()
	}()

	send := func() bool {
		data, err := json.Marshal(gin.H{"requests": recentSnapshot()})
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
			return false
		}
		c.Writer.Flush()
		return true
	}
	if !send() {
		return
	}
	done := c.Request.Context().Done()
	for {
		select {
		case <-done:
			return
		case <-ch:
			if !send() {
				return
			}
		}
	}
}

// firstUserText returns the first user message (best-effort preview); empty
// when the processor has no prompt yet.
func firstUserText(p *utils.ChatRequestProcessor) string {
	if p == nil {
		return ""
	}
	return p.Prompt.String()
}
