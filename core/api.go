package core

import (
	"bufio"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"claude2api/config"
	"claude2api/logger"
	"claude2api/model"
	"claude2api/utils"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/imroc/req/v3"
	tls "github.com/refraction-networking/utls"
)

type Client struct {
	SessionKey   string
	orgID        string
	client       *req.Client
	model        string
	defaultAttrs map[string]interface{}
}

type ResponseEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
	} `json:"content_block"`
	Delta struct {
		Type     string `json:"type"`
		Text     string `json:"text"`
		THINKING string `json:"thinking"`
		// partial_json
		PartialJSON string `json:"partial_json"`
	} `json:"delta"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

// toolUseInput is the JSON object carried by a tool_use input_json_delta. We
// keep only the code-bearing fields (file_text, widget_code, content) plus a
// couple of hints (language/type) for the fenced code block; command/path/
// description/filepaths metadata is dropped from the streamed output.
type toolUseInput struct {
	FileText   string `json:"file_text"`
	WidgetCode string `json:"widget_code"`
	Content    string `json:"content"`
	Language   string `json:"language"`
	Type       string `json:"type"`
	Path       string `json:"path"`
}

func detectToolLanguage(input toolUseInput) string {
	if input.Language != "" {
		return input.Language
	}
	if input.Type == "text/html" || input.Type == "html" {
		return "html"
	}
	if input.Type != "" && !strings.Contains(input.Type, "/") {
		return input.Type
	}
	if input.Path != "" {
		ext := strings.TrimPrefix(filepath.Ext(input.Path), ".")
		switch strings.ToLower(ext) {
		case "js", "jsx":
			return "javascript"
		case "ts", "tsx":
			return "typescript"
		case "py":
			return "python"
		case "rb":
			return "ruby"
		case "rs":
			return "rust"
		case "sh", "bash":
			return "bash"
		case "yml":
			return "yaml"
		case "":
			return "text"
		default:
			return strings.ToLower(ext)
		}
	}
	return "text"
}

// upstreamTool maps one OpenAI-style client tool to the claude.ai tool entry
// it names, or nil when there is no upstream equivalent (arbitrary function
// tools can't run on the web backend). Accepts both OpenAI shape
// {"type":"function","function":{"name":...}} and flat {"name":...} entries.
// ToolPrompt renders client function tools as instructions so the web model —
// which has no function-calling API — invokes them by emitting a fenced
// ```toolcall {"name":"...","arguments":{...}}``` block instead. Returns "" when
// there are no function tools to describe.
func ToolPrompt(clientTools []map[string]interface{}) string {
	var b strings.Builder
	for _, t := range clientTools {
		fn, ok := t["function"].(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := fn["name"].(string)
		if name == "" || strings.ToLower(name) == "web_search" {
			continue
		}
		desc, _ := fn["description"].(string)
		params, _ := json.Marshal(fn["parameters"])
		fmt.Fprintf(&b, "- %s: %s Schema: %s\n", name, desc, params)
	}
	if b.Len() == 0 {
		return ""
	}
	return "You have these tools; to call one reply with ONLY a fenced block ```toolcall {\"name\":\"<tool>\",\"arguments\":{...}}``` and no other text:\n" + b.String() + "\n"
}

func upstreamTool(t map[string]interface{}) map[string]interface{} {
	name := ""
	if fn, ok := t["function"].(map[string]interface{}); ok {
		name, _ = fn["name"].(string)
	}
	if name == "" {
		name, _ = t["name"].(string)
	}
	switch strings.ToLower(name) {
	case "web_search", "websearch":
		return map[string]interface{}{"type": "web_search_v0", "name": "web_search"}
	case "artifacts", "artifact":
		return map[string]interface{}{"type": "artifacts_v0", "name": "artifacts"}
	case "repl", "code_interpreter":
		return map[string]interface{}{"type": "repl_v0", "name": "repl"}
	}
	if typ, _ := t["type"].(string); strings.HasSuffix(typ, "_v0") {
		return map[string]interface{}{"type": typ, "name": name}
	}
	return nil
}

// resolveUpstreamTools converts client-requested tools to the claude.ai tool
// list. Empty (or all-unknown) client list keeps the defaults.
func resolveUpstreamTools(clientTools []map[string]interface{}, defaults []map[string]interface{}) []map[string]interface{} {
	if len(clientTools) == 0 {
		return defaults
	}
	var out []map[string]interface{}
	for _, t := range clientTools {
		if u := upstreamTool(t); u != nil {
			out = append(out, u)
		}
	}
	if len(out) == 0 {
		return defaults
	}
	return out
}

// stableDeviceID derives a per-session UUID so claude.ai sees a consistent
// "device" for each sessionKey, like the web app does for a browser.
func stableDeviceID(sessionKey string) string {
	h := sha256.Sum256([]byte("claude2api-device:" + sessionKey))
	u := uuid.UUID(h[:16])
	u[6] = (u[6] & 0x0f) | 0x40 // version 4
	u[8] = (u[8] & 0x3f) | 0x80 // RFC 4122 variant
	return u.String()
}

// randomTraceID returns a 32-hex-char W3C trace id
func randomTraceID() string {
	b := make([]byte, 16)
	_, _ = cryptorand.Read(b)
	return hex.EncodeToString(b)
}

// hexToDecString converts a hex chunk to its decimal string form (datadog ids)
func hexToDecString(hexStr string) string {
	v, err := strconv.ParseUint(hexStr, 16, 64)
	if err != nil {
		return "1"
	}
	return strconv.FormatUint(v, 10)
}

// ModelThinkingSpec mirrors the web client's thinking_by_model entry. For
// effort_and_mode models the client can pick an effort (low/medium/high/xhigh);
// for "mode" models there is no effort, only an extended-thinking toggle.
type ModelThinkingSpec struct {
	DefaultEffort string   // default effort if the id omits one
	Mode          string   // off | auto | extended
	Type          string   // effort_and_mode | mode
	Efforts       []string // selectable efforts exposed in /v1/models (empty for mode-only)
}

// modelThinkingSpecs is the per-model thinking config for the free tier
// (default_claude_ai). Only the three free models are listed. Keep order
// stable so /v1/models is predictable.
var modelThinkingSpecs = []struct {
	ID   string
	Spec ModelThinkingSpec
}{
	{"claude-sonnet-5", ModelThinkingSpec{
		DefaultEffort: "high", Mode: "auto", Type: "effort_and_mode",
		Efforts: []string{"low", "medium", "high"},
	}},
	{"claude-haiku-4-5-20251001", ModelThinkingSpec{
		Mode: "extended", Type: "mode",
	}},
	{"claude-sonnet-4-6", ModelThinkingSpec{
		DefaultEffort: "low", Mode: "off", Type: "effort_and_mode",
		Efforts: []string{"low", "medium", "high"},
	}},
}

// BootstrapModel is one entry of claude_ai_bootstrap_models_config on
// /api/bootstrap — the model list the web UI actually offers this account.
type BootstrapModel struct {
	Model       string                   `json:"model"`
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Inactive    bool                     `json:"inactive"`
	Overflow    bool                     `json:"overflow"`
	PaprikaModes []string                `json:"paprika_modes"`
	ThinkingModes []map[string]interface{} `json:"thinking_modes"`
	HardLimit   int                      `json:"hard_limit"`
}

// bootstrapModelsConfig holds the model list advertised by claude.ai, refreshed
// in the background. It degrades to the built-in list when the fetch fails.
var (
	bootstrapModels     []BootstrapModel
	bootstrapModelsAt   time.Time
	bootstrapModelsLock sync.RWMutex
)

const bootstrapModelsTTL = 6 * time.Hour

// modelThinkingFromBootstrap maps a web-ui model entry to the spec used to
// parse "<base>[-effort][-think]" ids and build /v1/models. Only extended
// thinking is exposed as a "-think" variant: no effort variants are derived
// here, since the web no longer advertises a per-effort selector.
func modelThinkingFromBootstrap(m BootstrapModel) (string, ModelThinkingSpec) {
	spec := ModelThinkingSpec{Mode: "auto", Type: "mode"}
	extended := false
	for _, p := range m.PaprikaModes {
		if p == "extended" {
			extended = true
		}
	}
	if !extended {
		spec.Mode = "off"
	}
	return m.Model, spec
}

// FetchBootstrapModels asks claude.ai for the model list the account can
// actually use (/api/bootstrap → claude_ai_bootstrap_models_config) and caches
// it. Empty result means the fetch failed and the built-in list stays in force.
func FetchBootstrapModels(sessionKey, cookie, deviceID string) []BootstrapModel {
	bootstrapModelsLock.Lock()
	defer bootstrapModelsLock.Unlock()

	if time.Since(bootstrapModelsAt) < bootstrapModelsTTL && len(bootstrapModels) > 0 {
		return bootstrapModels
	}

	cl := req.C().ImpersonateChrome().SetTLSFingerprint(tls.HelloChrome_133).SetTimeout(30 * time.Second)
	if config.ConfigInstance.Proxy != "" {
		cl.SetProxyURL(config.ConfigInstance.Proxy)
	}
	if cookie == "" {
		cl.SetCommonCookies(
			&http.Cookie{Name: "sessionKey", Value: sessionKey},
			&http.Cookie{Name: "sessionKeyV3", Value: sessionKey},
		)
	} else {
		cl.SetCommonHeader("Cookie", cookie)
	}
	if deviceID != "" {
		cl.SetCommonHeader("anthropic-device-id", deviceID)
	} else {
		cl.SetCommonHeader("anthropic-device-id", stableDeviceID(sessionKey))
	}

	resp, err := cl.R().
		SetHeader("referer", "https://claude.ai/new").
		SetHeader("accept", "application/json").
		Get("https://claude.ai/api/bootstrap")
	if err != nil {
		logger.Error(fmt.Sprintf("Failed to fetch bootstrap model list: %v", err))
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		logger.Error(fmt.Sprintf("Bootstrap fetch returned %d", resp.StatusCode))
		return nil
	}

	var boot struct {
		Account struct {
			Memberships []struct {
				Organization struct {
					Models []BootstrapModel `json:"claude_ai_bootstrap_models_config"`
				} `json:"organization"`
			} `json:"memberships"`
		} `json:"account"`
	}
	if err := json.Unmarshal(resp.Bytes(), &boot); err != nil {
		logger.Error(fmt.Sprintf("Failed to parse bootstrap response: %v", err))
		return nil
	}
	var out []BootstrapModel
	for _, m := range boot.Account.Memberships {
		out = append(out, m.Organization.Models...)
	}
	if len(out) == 0 {
		return nil
	}
	bootstrapModels = out
	bootstrapModelsAt = time.Now()
	logger.Info(fmt.Sprintf("Fetched %d models from claude.ai", len(out)))
	return out
}

// BootstrapModelIDs derives the /v1/models ids from the cached web-ui model
// list: one base id per active model plus a "-think" variant when the web UI
// offers extended thinking for it. ok=false when nothing has been fetched yet.
func BootstrapModelIDs() ([]string, bool) {
	bootstrapModelsLock.RLock()
	defer bootstrapModelsLock.RUnlock()
	if len(bootstrapModels) == 0 {
		return nil, false
	}
	var out []string
	for _, m := range bootstrapModels {
		if m.Inactive {
			continue
		}
		out = append(out, m.Model)
		extended := false
		for _, p := range m.PaprikaModes {
			if p == "extended" {
				extended = true
			}
		}
		if extended {
			out = append(out, m.Model+"-think")
		}
	}
	return out, true
}

// ModelIDs returns the list of model ids exposed in /v1/models. For
// effort_and_mode models every selectable effort is a separate id, each with a
// "-think" variant that forces extended thinking on. Mode-only models expose a
// single base id plus a "-think" variant.
func ModelIDs() []string {
	var out []string
	for _, m := range modelThinkingSpecs {
		if len(m.Spec.Efforts) > 0 {
			for _, eff := range m.Spec.Efforts {
				out = append(out, m.ID+"-"+eff)
				out = append(out, m.ID+"-"+eff+"-think")
			}
		} else {
			out = append(out, m.ID)
			out = append(out, m.ID+"-think")
		}
	}
	return out
}

// specForModel looks up the thinking spec for a (base) model id. ok=false when
// the model is not in the table; callers fall back to medium/auto.
func specForModel(baseModel string) (ModelThinkingSpec, bool) {
	for _, m := range modelThinkingSpecs {
		if m.ID == baseModel {
			return m.Spec, true
		}
	}
	return ModelThinkingSpec{}, false
}

// knownEfforts returns the set of valid effort suffixes for a model (for
// suffix parsing).
func knownEfforts(spec ModelThinkingSpec) map[string]bool {
	set := map[string]bool{}
	for _, e := range spec.Efforts {
		set[e] = true
	}
	return set
}

// ResolveThinking parses a model id of the form
// "<base>[-effort][-think]" into the base model id, effort, thinking_mode and
// paprika_mode to send in the completion body. The effort suffix (when the
// model supports it) overrides the spec default; "-think" forces extended
// thinking on (paprika_mode="extended"). Unknown models fall back to
// effort=medium, mode=auto.
func ResolveThinking(model string) (baseModel, effort, thinkingMode, paprikaMode string) {
	thinking := strings.HasSuffix(model, "-think")
	rest := strings.TrimSuffix(model, "-think")

	// Find the matching spec by checking whether `rest` is "<base>" or
	// "<base>-<effort>". Try longest base first so e.g. claude-sonnet-4-6 is
	// matched before its "-6" tail is mistaken for an effort.
	var base, eff string
	var spec ModelThinkingSpec
	var ok bool
	for _, m := range modelThinkingSpecs {
		if rest == m.ID {
			base, spec, ok = m.ID, m.Spec, true
			break
		}
		if strings.HasPrefix(rest, m.ID+"-") {
			tail := strings.TrimPrefix(rest, m.ID+"-")
			if knownEfforts(m.Spec)[tail] {
				base, spec, eff, ok = m.ID, m.Spec, tail, true
				break
			}
		}
	}
	if !ok {
		// Unknown model: legacy default (medium/auto), think → extended.
		effort, thinkingMode = "medium", "auto"
		if thinking {
			paprikaMode = "extended"
		}
		return rest, effort, thinkingMode, paprikaMode
	}
	baseModel = base
	effort = eff
	if effort == "" {
		effort = spec.DefaultEffort
	}
	if effort == "" {
		effort = "medium"
	}
	thinkingMode = spec.Mode
	// "-think" forces extended thinking on regardless of the spec's default mode.
	if thinking {
		paprikaMode = "extended"
	}
	return baseModel, effort, thinkingMode, paprikaMode
}

func NewClient(sessionKey string, proxy string, model string) *Client {
	return NewClientWithCookie(sessionKey, proxy, model, "", "")
}

// NewClientWithCookie is NewClient plus an optional raw browser Cookie header
// (cf, routingHint, …) and the matching anthropic device id for that browser.
// Empty cookie means only sessionKey(s) are sent; empty deviceID falls back to
// the per-session stable id so the upstream still sees a consistent device.
func NewClientWithCookie(sessionKey string, proxy string, model string, cookie string, deviceID string) *Client {
	// ImpersonateChrome() defaults to the utls Chrome 120 TLS fingerprint,
	// which lags the current Chrome major version signed by actual browsers and
	// can read as an older/emulated client. Override it with the newest client
	// hello available in utls (Chrome 133) to keep the TLS profile fresh.
	client := req.C().ImpersonateChrome().SetTLSFingerprint(tls.HelloChrome_133).SetTimeout(time.Minute * 30)
	// claude.ai often takes >10s to respond under load; a short header timeout
	// causes intermittent request timeouts
	client.Transport.SetResponseHeaderTimeout(time.Second * 120)
	if proxy != "" {
		client.SetProxyURL(proxy)
	}
	// Set common headers - anthropic-device-id and the datadog trace headers
	// mirror what a real browser session sends; without them claude.ai treats
	// requests as bot traffic with much stricter rate limits.
	// ImpersonateChrome() injects sec-fetch-mode: navigate + UA/sec-ch-ua of
	// Chrome on macOS; that fingerprint is for a full-page navigation, NOT a
	// same-origin fetch to the JSON API. The web UI sends these API calls as
	// same-origin POSTs, so we must override those fields to match - otherwise
	// Cloudflare flags the request as bot traffic.
	headers := map[string]string{
		"accept":                         "text/event-stream, text/event-stream",
		"accept-language":                "en-US,en;q=0.9",
		"anthropic-client-platform":      "web_claude_ai",
		"content-type":                   "application/json",
		"origin":                         "https://claude.ai",
		"priority":                       "u=1, i",
		"sec-ch-ua-platform":             `"Windows"`,
		"sec-fetch-site":                 "same-origin",
		"sec-fetch-mode":                 "cors",
		"sec-fetch-dest":                 "empty",
		"sec-fetch-user":                 "?0",
		"user-agent":                     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}
	for key, value := range headers {
		client.SetCommonHeader(key, value)
	}
	if deviceID != "" {
		client.SetCommonHeader("anthropic-device-id", deviceID)
	} else {
		client.SetCommonHeader("anthropic-device-id", stableDeviceID(sessionKey))
	}
	traceID := randomTraceID()
	client.SetCommonHeader("traceparent", fmt.Sprintf("00-%s-%s-01", traceID, traceID[len(traceID)-16:]))
	client.SetCommonHeader("x-datadog-trace-id", hexToDecString(traceID[16:32]))
	client.SetCommonHeader("x-datadog-parent-id", hexToDecString(traceID[len(traceID)-16:]))
	client.SetCommonHeader("x-datadog-sampling-priority", "1")
	client.SetCommonHeader("x-datadog-origin", "rum")
	// Set cookies - sessionKeyV3 mirrors what the web app sends alongside
	// sessionKey; both carry the same value. When an extra cookie header is
	// configured (cf_bm, routingHint, …), it is merged with the session keys so
	// the upstream sees the same browser state as the real web app.
	if cookie == "" {
		client.SetCommonCookies(
			&http.Cookie{Name: "sessionKey", Value: sessionKey},
			&http.Cookie{Name: "sessionKeyV3", Value: sessionKey},
		)
	} else {
		// The cookie file already carries sessionKey/sessionKeyV3 values; only
		// append them if the file lacks them (key mismatch from env sessions).
		cookie = strings.TrimSpace(cookie)
		if !strings.Contains(cookie, "sessionKey=") {
			cookie += "; sessionKey=" + sessionKey
		}
		if !strings.Contains(cookie, "sessionKeyV3=") {
			cookie += "; sessionKeyV3=" + sessionKey
		}
		client.SetCommonHeader("Cookie", cookie)
	}
	// Create default client with session key
	c := &Client{
		SessionKey: sessionKey,
		client:     client,
		model:      model,
		defaultAttrs: map[string]interface{}{
			"personalized_styles": []map[string]interface{}{
				{
					"type":       "default",
					"key":        "Default",
					"name":       "Normal",
					"nameKey":    "normal_style_name",
					"prompt":     "Normal",
					"summary":    "Default responses from Claude",
					"summaryKey": "normal_style_summary",
					"isDefault":  true,
				},
			},
			"tools": []map[string]interface{}{
				{
					"type": "web_search_v0",
					"name": "web_search",
				},
				{"type": "artifacts_v0", "name": "artifacts"},
				{"type": "repl_v0", "name": "repl"},
			},
			"parent_message_uuid": "00000000-0000-4000-8000-000000000000",
			"attachments":         []interface{}{},
			"files":               []interface{}{},
			"sync_sources":        []interface{}{},
			"rendering_mode":      "messages",
			"timezone":            "America/Los_Angeles",
		},
	}
	return c
}

// SetTools overrides the default upstream tool list with the client-requested
// one. Empty (or all-unknown) input keeps the defaults.
func (c *Client) SetTools(clientTools []map[string]interface{}) {
	if len(clientTools) == 0 {
		return
	}
	defaults, _ := c.defaultAttrs["tools"].([]map[string]interface{})
	c.defaultAttrs["tools"] = resolveUpstreamTools(clientTools, defaults)
}

// SetOrgID sets the organization ID for the client
func (c *Client) SetOrgID(orgID string) {
	c.orgID = orgID
}
func (c *Client) GetOrgID() (string, error) {
	url := "https://claude.ai/api/organizations"
	resp, err := c.client.R().
		SetHeader("referer", "https://claude.ai/new").
		Get(url)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	type OrgResponse []struct {
		ID            int    `json:"id"`
		UUID          string `json:"uuid"`
		Name          string `json:"name"`
		RateLimitTier string `json:"rate_limit_tier"`
	}

	var orgs OrgResponse
	if err := json.Unmarshal(resp.Bytes(), &orgs); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if len(orgs) == 0 {
		return "", errors.New("no organizations found")
	}
	if len(orgs) == 1 {
		return orgs[0].UUID, nil
	}
	for _, org := range orgs {
		if org.RateLimitTier == "default_claude_ai" || org.RateLimitTier == "default_claude_max_20x" || org.RateLimitTier == "default_raven_enterprise" {
			return org.UUID, nil
		}
	}
	return "", errors.New("no default organization found")

}

// AccountInfo is the subset of /api/account we use to label an account in the
// UI (email) and classify it (rate_limit_tier on the org).
type AccountInfo struct {
	EmailAddress string `json:"email_address"`
}

// GetAccountEmail calls /api/account and returns the account's email address.
// Used as the human-readable display name when a cookie account has no label.
func (c *Client) GetAccountEmail() (string, error) {
	resp, err := c.client.R().
		SetHeader("referer", "https://claude.ai/").
		SetHeader("accept", "application/json").
		Get("https://claude.ai/api/account")
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	var acct AccountInfo
	if err := json.Unmarshal(resp.Bytes(), &acct); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	if acct.EmailAddress == "" {
		return "", errors.New("no email_address in account response")
	}
	return acct.EmailAddress, nil
}

// CreateConversation creates a new conversation and returns its UUID
func (c *Client) CreateConversation() (string, error) {
	if c.orgID == "" {
		return "", errors.New("organization ID not set")
	}
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations", c.orgID)
	// Resolve thinking effort/mode for this model; strip the "-think" suffix
	// and set paprika_mode to "extended" only when the resolved mode wants it.
	baseModel, _, _, paprikaMode := ResolveThinking(c.model)
	c.model = baseModel
	paprikaVal := interface{}(nil)
	if paprikaMode != "" {
		paprikaVal = paprikaMode
	}
	if err := c.UpdateUserSetting("paprika_mode", paprikaVal); err != nil {
		logger.Error(fmt.Sprintf("Failed to update paprika_mode: %v", err))
	}
	requestBody := map[string]interface{}{
		"model":                            c.model,
		"uuid":                             uuid.New().String(),
		"name":                             "",
		"include_conversation_preferences": true,
	}

	resp, err := c.client.R().
		SetHeader("referer", "https://claude.ai/new").
		SetBody(requestBody).
		Post(url)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	var result map[string]interface{}
	// logger.Info(fmt.Sprintf("create conversation response: %s", resp.String()))
	if err := json.Unmarshal(resp.Bytes(), &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}
	logger.Info(fmt.Sprintf("create conversation response: %s", resp.String()))
	uuid, ok := result["uuid"].(string)
	if !ok {
		return "", errors.New("conversation UUID not found in response")
	}
	return uuid, nil
}

// SendMessageWithCreate mirrors the browser flow exactly: a single POST to
// /completion with create_conversation_params embedded, so claude.ai creates
// the conversation inline. Halves the number of upstream requests per message
// compared to the create-then-send two-step flow.
func (c *Client) SendMessageWithCreate(message string, stream bool, gc *gin.Context) (string, int, error) {
	if c.orgID == "" {
		return "", 500, errors.New("organization ID not set")
	}

	model, effort, thinkingMode, paprikaMode := ResolveThinking(c.model)
	paprika := interface{}(nil)
	if paprikaMode != "" {
		paprika = paprikaMode
	}

	requestBody := c.defaultAttrs
	requestBody["prompt"] = message
	requestBody["model"] = model
	requestBody["locale"] = "en-US"
	requestBody["thinking_mode"] = thinkingMode
	requestBody["effort"] = effort
	requestBody["turn_message_uuids"] = map[string]string{
		"human_message_uuid":     uuid.New().String(),
		"assistant_message_uuid": uuid.New().String(),
	}
	requestBody["create_conversation_params"] = map[string]interface{}{
		"name":                             "",
		"model":                            model,
		"include_conversation_preferences": true,
		"paprika_mode":                     paprika,
		"is_temporary":                     false,
	}

	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations/%s/completion",
		c.orgID, newConversationID())

	resp, err := c.client.R().DisableAutoReadResponse().
		SetHeader("referer", "https://claude.ai/new").
		SetHeader("accept", "text/event-stream, text/event-stream").
		SetHeader("anthropic-client-platform", "web_claude_ai").
		SetHeader("cache-control", "no-cache").
		SetBody(requestBody).
		Post(url)
	if err != nil {
		return "", 500, fmt.Errorf("request failed: %w", err)
	}
	logger.Info(fmt.Sprintf("Claude response status code: %d", resp.StatusCode))
	if resp.StatusCode == http.StatusTooManyRequests {
		return "", http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded")
	}
	if resp.StatusCode != http.StatusOK {
		return "", resp.StatusCode, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	c.model = model // remember trimmed model for cleanup calls
	if err := c.HandleResponse(resp.Body, stream, gc); err != nil {
		return "", 500, err
	}
	return "", 200, nil
}

func newConversationID() string {
	return uuid.New().String()
}

// PeekNewConversationID generates the conversation UUID that
// SendMessageWithCreate will use, so callers can reference it for cleanup
// before the request completes.
func (c *Client) PeekNewConversationID() string {
	return newConversationID()
}

// SendMessage sends a message to a conversation and returns the status and response
func (c *Client) SendMessage(conversationID string, message string, stream bool, gc *gin.Context) (int, error) {
	if c.orgID == "" {
		return 500, errors.New("organization ID not set")
	}
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations/%s/completion",
		c.orgID, conversationID)
	// Create request body with default attributes
	model, effort, thinkingMode, paprikaMode := ResolveThinking(c.model)
	paprika := interface{}(nil)
	if paprikaMode != "" {
		paprika = paprikaMode
	}
	requestBody := c.defaultAttrs
	requestBody["prompt"] = message
	requestBody["model"] = model
	requestBody["locale"] = "en-US"
	requestBody["thinking_mode"] = thinkingMode
	requestBody["effort"] = effort
	requestBody["turn_message_uuids"] = map[string]string{
		"human_message_uuid":     uuid.New().String(),
		"assistant_message_uuid": uuid.New().String(),
	}
	requestBody["create_conversation_params"] = map[string]interface{}{
		"name":                             "",
		"model":                            model,
		"include_conversation_preferences": true,
		"paprika_mode":                     paprika,
		"is_temporary":                     false,
	}
	// Set up streaming response
	resp, err := c.client.R().DisableAutoReadResponse().
		SetHeader("referer", fmt.Sprintf("https://claude.ai/chat/%s", conversationID)).
		SetHeader("accept", "text/event-stream, text/event-stream").
		SetHeader("anthropic-client-platform", "web_claude_ai").
		SetHeader("cache-control", "no-cache").
		SetBody(requestBody).
		Post(url)
	if err != nil {
		return 500, fmt.Errorf("request failed: %w", err)
	}
	logger.Info(fmt.Sprintf("Claude response status code: %d", resp.StatusCode))
	if resp.StatusCode == http.StatusTooManyRequests {
		return http.StatusTooManyRequests, fmt.Errorf("rate limit exceeded")
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	if err := c.HandleResponse(resp.Body, stream, gc); err != nil {
		return 500, err
	}
	return 200, nil
}

// HandleResponse converts Claude's SSE format to OpenAI format and writes to the response writer
// Returns nil if the request completed successfully (or the error came after content
// was already streamed), or an error if it failed early enough to allow a clean retry.
func (c *Client) HandleResponse(body io.ReadCloser, stream bool, gc *gin.Context) error {
	defer body.Close()
	// Set headers for streaming. The status is intentionally NOT committed
	// here: writing 200 before reading the first event would lock the response
	// in even if claude.ai answers with an error event before any content
	// (e.g. a rate limit). net/http commits status 200 automatically on the
	// first real Write below, so an early error can still propagate up and let
	// the caller retry another session.
	if stream {
		gc.Writer.Header().Set("Content-Type", "text/event-stream")
		gc.Writer.Header().Set("Cache-Control", "no-cache")
		gc.Writer.Header().Set("Connection", "keep-alive")
	}
	scanner := bufio.NewScanner(body)
	// SSE lines can be very large (artifacts / long code blocks); the default
	// 64KB buffer aborts mid-stream with "token too long"
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	clientDone := gc.Request.Context().Done()
	// Keep track of the full response for the final message
	thinkingShown := false
	res_all_text := ""
	partial_json_shown := false
	languageStr := "md"
	// Holds back a leading ```toolcall block so it streams as tool_calls, not
	// raw JSON content.
	toolFilter := &model.StreamToolFilter{}
	// pendingJSON accumulates a tool_use input_json_delta object until it parses
	// as a complete JSON object, so we can extract the code field intact.
	pendingJSON := ""
	for scanner.Scan() {
		select {
		case <-clientDone:
			// 客户端已断开连接，清理资源并退出
			logger.Info("Client closed connection")
			return nil
		default:
			// 继续处理响应
		}
		line := scanner.Text()
		// logger.Info(fmt.Sprintf("Claude SSE line: %s", line))
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := line[6:]
		var event ResponseEvent
		if err := json.Unmarshal([]byte(data), &event); err == nil {
			if event.Type == "error" && event.Error.Message != "" {
				if res_all_text != "" {
					// Content was already streamed to the client; append the
					// error message rather than failing the whole request
					model.ReturnOpenAIResponse("\n\n"+event.Error.Message, stream, gc)
					return nil
				}
				// Nothing streamed yet - propagate so the caller can retry
				return errors.New(event.Error.Message)
			}
			if event.Type == "content_block_stop" {
				// Flush any tool_use input still pending at block end. By now
				// the object should parse; if it does and carries a code field,
				// stream that. Otherwise drop the metadata.
				if pendingJSON != "" {
					var input toolUseInput
					if err := json.Unmarshal([]byte(pendingJSON), &input); err == nil {
						code := input.FileText
						if code == "" {
							code = input.WidgetCode
						}
						if code == "" {
							code = input.Content
						}
						if code != "" {
							out := code
							if !partial_json_shown {
								languageStr = detectToolLanguage(input)
								out = "\n```" + languageStr + "\n" + out
								partial_json_shown = true
							}
							res_all_text += out
							if stream {
								model.ReturnOpenAIResponse(out, stream, gc)
							}
						}
					}
					pendingJSON = ""
					languageStr = "md"
				}
				res_text := ""
				if thinkingShown {
					res_text = "</think>\n"
					thinkingShown = false
				}
				if partial_json_shown {
					res_text = "\n```\n"
					partial_json_shown = false
				}
				res_all_text += res_text
				if !stream {
					continue
				}
				model.ReturnOpenAIResponse(res_text, stream, gc)
				continue
			}
			if event.Delta.Type == "text_delta" && event.Delta.Text != "" {
				res_text := event.Delta.Text
				res_all_text += res_text
				if !stream {
					continue
				}
				toolFilter.Feed(res_text, stream, gc)
				continue
			}
			if event.Delta.Type == "thinking_delta" {
				res_text := event.Delta.THINKING
				if !thinkingShown {
					res_text = "<think> " + res_text
					thinkingShown = true
				}
				res_all_text += res_text
				if !stream {
					continue
				}
				model.ReturnOpenAIResponse(res_text, stream, gc)
				continue
			}
			if event.Delta.Type == "input_json_delta" {
				// input_json_delta carries a JSON-escaped fragment of a tool_use
				// input object, e.g. {"command":"...","file_text":"<code>"}.
				// We only want the code-bearing field (file_text / widget_code /
				// content) streamed to the client, not the command/path/description
				// metadata. Fragments can split escapes and multibyte chars, so we
				// accumulate until the object parses and then stream the code field
				// value (decoded, UTF-8 and quotes intact).
				pendingJSON += event.Delta.PartialJSON
				var input toolUseInput
				if err := json.Unmarshal([]byte(pendingJSON), &input); err != nil {
					// Object not complete yet (cut mid-escape/multibyte) — wait.
					continue
				}
				// Pick the code field present in this tool input.
				code := input.FileText
				if code == "" {
					code = input.WidgetCode
				}
				if code == "" {
					code = input.Content
				}
				if code == "" {
					// No code field — this is a non-code tool (read_me, web_search
					// params, …): drop the metadata entirely.
					pendingJSON = ""
					continue
				}
				out := code
				// Open a fenced code block before the first artifact output.
				if !partial_json_shown {
					languageStr = detectToolLanguage(input)
					out = "\n```" + languageStr + "\n" + out
					partial_json_shown = true
				}
				res_all_text += out
				pendingJSON = ""
				if stream {
					model.ReturnOpenAIResponse(out, stream, gc)
				}
				continue
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}
	// claude.ai never reports token counts, so estimate the output side from
	// the assembled text for the usage object and the dashboard log.
	gc.Set("CompletionTokens", utils.EstimateCompletionTokens(res_all_text))
	if !stream {
		model.ResponseWithTools(res_all_text, stream, gc)
	} else {
		// Flush any held-back text/toolcall block, then the end marker.
		toolFilter.Finish(gc)
		gc.Writer.Write([]byte("data: [DONE]\n\n"))
		gc.Writer.Flush()
	}

	return nil
}

// DeleteConversation deletes a conversation by ID
func (c *Client) DeleteConversation(conversationID string) error {
	if c.orgID == "" {
		return errors.New("organization ID not set")
	}
	url := fmt.Sprintf("https://claude.ai/api/organizations/%s/chat_conversations/%s",
		c.orgID, conversationID)
	requestBody := map[string]string{
		"uuid": conversationID,
	}
	resp, err := c.client.R().
		SetHeader("referer", fmt.Sprintf("https://claude.ai/chat/%s", conversationID)).
		SetBody(requestBody).
		Delete(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return nil
}

// UploadFile uploads files to Claude and adds them to the client's default attributes
// fileData should be in the format: data:image/jpeg;base64,/9j/4AA...
func (c *Client) UploadFile(fileData []string) error {
	if c.orgID == "" {
		return errors.New("organization ID not set")
	}
	if len(fileData) == 0 {
		return errors.New("empty file data")
	}

	// Initialize files array in default attributes if it doesn't exist
	if _, ok := c.defaultAttrs["files"]; !ok {
		c.defaultAttrs["files"] = []interface{}{}
	}

	// Process each file
	for _, fd := range fileData {
		if fd == "" {
			continue // Skip empty entries
		}

		// Parse the base64 data
		parts := strings.SplitN(fd, ",", 2)
		if len(parts) != 2 {
			return errors.New("invalid file data format")
		}

		// Get the content type from the data URI
		metaParts := strings.SplitN(parts[0], ":", 2)
		if len(metaParts) != 2 {
			return errors.New("invalid content type in file data")
		}

		metaInfo := strings.SplitN(metaParts[1], ";", 2)
		if len(metaInfo) != 2 || metaInfo[1] != "base64" {
			return errors.New("invalid encoding in file data")
		}

		contentType := metaInfo[0]

		// Decode the base64 data
		fileBytes, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return fmt.Errorf("failed to decode base64 data: %w", err)
		}

		// Determine filename based on content type
		var filename string
		switch contentType {
		case "image/jpeg":
			filename = "image.jpg"
		case "image/png":
			filename = "image.png"
		case "application/pdf":
			filename = "document.pdf"
		default:
			filename = "file"
		}

		// Create the upload URL
		url := fmt.Sprintf("https://claude.ai/api/%s/upload", c.orgID)

		// Create a multipart form request
		resp, err := c.client.R().
			SetHeader("referer", "https://claude.ai/new").
			SetHeader("anthropic-client-platform", "web_claude_ai").
			SetFileBytes("file", filename, fileBytes).
			SetContentType("multipart/form-data").
			Post(url)

		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, resp.String())
		}

		// Parse the response
		var result struct {
			FileUUID string `json:"file_uuid"`
		}

		if err := json.Unmarshal(resp.Bytes(), &result); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if result.FileUUID == "" {
			return errors.New("file UUID not found in response")
		}

		// Add file to default attributes
		c.defaultAttrs["files"] = append(c.defaultAttrs["files"].([]interface{}), result.FileUUID)
	}

	return nil
}

func (c *Client) SetBigContext(context string) {
	c.defaultAttrs["attachments"] = []map[string]interface{}{
		{
			"file_name":         "context.txt",
			"file_type":         "text/plain",
			"file_size":         len(context),
			"extracted_content": context,
		},
	}

}

// / UpdateUserSetting updates a single user setting on Claude.ai while preserving all other settings
func (c *Client) UpdateUserSetting(key string, value interface{}) error {
	url := "https://claude.ai/api/account?statsig_hashing_algorithm=djb2"

	// Default settings structure with all possible fields
	settings := map[string]interface{}{
		"input_menu_pinned_items":          nil,
		"has_seen_mm_examples":             nil,
		"has_seen_starter_prompts":         nil,
		"has_started_claudeai_onboarding":  true,
		"has_finished_claudeai_onboarding": true,
		"dismissed_claudeai_banners":       []interface{}{},
		"dismissed_artifacts_announcement": nil,
		"preview_feature_uses_artifacts":   nil,
		"preview_feature_uses_latex":       nil,
		"preview_feature_uses_citations":   nil,
		"preview_feature_uses_harmony":     nil,
		"enabled_artifacts_attachments":    true,
		"enabled_turmeric":                 nil,
		"enable_chat_suggestions":          nil,
		"dismissed_artifact_feedback_form": nil,
		"enabled_mm_pdfs":                  nil,
		"enabled_gdrive":                   nil,
		"enabled_bananagrams":              nil,
		"enabled_gdrive_indexing":          nil,
		"enabled_web_search":               true,
		"enabled_compass":                  nil,
		"enabled_sourdough":                nil,
		"enabled_foccacia":                 nil,
		"dismissed_claude_code_spotlight":  nil,
		"enabled_geolocation":              nil,
		"enabled_mcp_tools":                nil,
		"paprika_mode":                     nil,
		"enabled_monkeys_in_a_barrel":      nil,
	}

	// Update the specified setting
	if _, exists := settings[key]; exists {
		settings[key] = value
		logger.Info(fmt.Sprintf("Updating setting %s to %v", key, value))
	} else {
		return fmt.Errorf("unknown setting key: %s", key)
	}

	// Create request body
	requestBody := map[string]interface{}{
		"settings": settings,
	}

	// Make the request
	resp, err := c.client.R().
		SetHeader("referer", "https://claude.ai/new").
		SetHeader("origin", "https://claude.ai").
		SetHeader("anthropic-client-platform", "web_claude_ai").
		SetHeader("cache-control", "no-cache").
		SetHeader("pragma", "no-cache").
		SetHeader("priority", "u=1, i").
		SetBody(requestBody).
		Put(url)

	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != 202 {
		return fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, resp.String())
	}

	// logger.Info(fmt.Sprintf("Successfully updated user setting %s: %s", key, resp.String()))
	return nil
}
