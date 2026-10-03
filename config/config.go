package config

import (
	"claude2api/logger"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/yaml.v3"
)

type SessionInfo struct {
	SessionKey string `yaml:"sessionKey"`
	OrgID      string `yaml:"orgID"`
	// DisplayName is an optional human label for the account (e.g. an email),
	// shown in the UI instead of the raw session key.
	DisplayName string `yaml:"displayName,omitempty"`
	// Disabled flags the account as manually powered off (persisted to
	// accounts.json); disabled accounts are skipped by round-robin.
	Disabled bool `yaml:"-" json:"-"`
	// ExtraCookie is the full browser Cookie header (cf_bm, routingHint, …)
	// attached to upstream requests alongside sessionKey. Loaded from a cookie
	// file/account when present.
	ExtraCookie string `yaml:"extraCookie,omitempty"`
	// CookieJSON keeps the raw cookie array (for re-export to accounts.json).
	// Not serialized by config loaders.
	CookieJSON []CookieFile `yaml:"-" json:"-"`
	// DeviceID mirrors the browser anthropic-device-id cookie when using a
	// cookie file, so upstream sees the same device as the real browser.
	DeviceID string `yaml:"deviceId,omitempty"`
	// mu serializes requests on one session: concurrent requests would
	// otherwise overwrite each other's paprika_mode (think mode) settings.
	// Pointer so that SessionInfo copies share the same lock.
	mu *sync.Mutex `yaml:"-"`
}

func (s *SessionInfo) Lock() {
	s.mu.Lock()
}

func (s *SessionInfo) Unlock() {
	s.mu.Unlock()
}

// NewSessionInfo creates a SessionInfo with an initialized lock, for sessions
// that are not loaded from config (e.g. extracted from request headers)
func NewSessionInfo(sessionKey, orgID string) *SessionInfo {
	return &SessionInfo{SessionKey: sessionKey, OrgID: orgID, mu: &sync.Mutex{}}
}

// NewSessionInfoWithCookie is NewSessionInfo with an optional Cookie header,
// device id and display name, for sessions configured from a full browser cookie.
func NewSessionInfoWithCookie(sessionKey, orgID, displayName, extraCookie, deviceID string) *SessionInfo {
	s := NewSessionInfo(sessionKey, orgID)
	s.DisplayName = displayName
	s.ExtraCookie = extraCookie
	s.DeviceID = deviceID
	return s
}

type SessionRagen struct {
	Index int
	Mutex sync.Mutex
}

type Config struct {
	Sessions               []SessionInfo `yaml:"sessions"`
	Address                string        `yaml:"address"`
	APIKey                 string        `yaml:"apiKey"`
	Proxy                  string        `yaml:"proxy"`
	ChatDelete             bool          `yaml:"chatDelete"`
	MaxChatHistoryLength   int           `yaml:"maxChatHistoryLength"`
	NoRolePrefix           bool          `yaml:"noRolePrefix"`
	PromptDisableArtifacts bool          `yaml:"promptDisableArtifacts"`
	EnableMirrorApi        bool          `yaml:"enableMirrorApi"`
	MirrorApiPrefix        string        `yaml:"mirrorApiPrefix"`
	// EnableTools forwards client-requested tools to the model (web_search,
	// artifacts, repl) and exposes client function tools as OpenAI tool_calls.
	EnableTools            bool          `yaml:"enableTools"`
	EnableGateway          bool          `yaml:"enableGateway"` // phục vụ giao diện claude.ai tại localhost
	GatewayKey             string        `yaml:"gatewayKey"`    // sessionKey gateway ưu tiên dùng; rỗng = tự chọn
	RwMutx                 sync.RWMutex  `yaml:"-"`             // 不从YAML加载
}

// 解析 SESSIONS 格式的环境变量
func parseSessionEnv(envValue string) []SessionInfo {
	if envValue == "" {
		return []SessionInfo{}
	}
	var sessions []SessionInfo
	for _, pair := range strings.Split(envValue, ",") {
		if pair == "" {
			continue
		}
		parts := strings.Split(pair, ":")
		orgID := ""
		if len(parts) > 1 {
			orgID = parts[1]
		}
		sessions = append(sessions, *NewSessionInfoWithCookie(parts[0], orgID, "", "", ""))
	}
	return sessions
}

// 根据模型选择合适的 session
func (c *Config) GetSessionForModel(idx int) (*SessionInfo, error) {
	if len(c.Sessions) == 0 || idx < 0 || idx >= len(c.Sessions) {
		return nil, fmt.Errorf("invalid session index: %d", idx)
	}
	c.RwMutx.RLock()
	defer c.RwMutx.RUnlock()
	return &c.Sessions[idx], nil
}

// ToolsEnabled reports whether tool support is on (safe under the config lock).
func (c *Config) ToolsEnabled() bool {
	c.RwMutx.RLock()
	defer c.RwMutx.RUnlock()
	return c.EnableTools
}

// SetGatewayKey stores the preferred gateway session key
func (c *Config) SetGatewayKey(key string) {
	c.RwMutx.Lock()
	defer c.RwMutx.Unlock()
	c.GatewayKey = key
}

// GetGatewayKey returns the preferred gateway session key
func (c *Config) GetGatewayKey() string {
	c.RwMutx.RLock()
	defer c.RwMutx.RUnlock()
	return c.GatewayKey
}

// AddSession appends a new session key if not already present. Returns true when added.
func (c *Config) AddSession(sessionKey string) bool {
	c.RwMutx.Lock()
	defer c.RwMutx.Unlock()
	for i := range c.Sessions {
		if c.Sessions[i].SessionKey == sessionKey {
			return false
		}
	}
	c.Sessions = append(c.Sessions, *NewSessionInfo(sessionKey, ""))
	return true
}

// RemoveSession deletes the session with the given key. Returns true when removed.
func (c *Config) RemoveSession(sessionKey string) bool {
	c.RwMutx.Lock()
	defer c.RwMutx.Unlock()
	for i := range c.Sessions {
		if c.Sessions[i].SessionKey == sessionKey {
			c.Sessions = append(c.Sessions[:i], c.Sessions[i+1:]...)
			return true
		}
	}
	return false
}

// AddSessionFromCookies builds a SessionInfo from a cookie array and appends
// it to the pool if the underlying sessionKey is not already present.
func (c *Config) AddSessionFromCookies(name string, cookies []CookieFile) (SessionInfo, bool) {
	s, ok := buildSessionFromCookies(name, cookies)
	if !ok {
		return SessionInfo{}, false
	}
	c.RwMutx.Lock()
	defer c.RwMutx.Unlock()
	for i := range c.Sessions {
		if c.Sessions[i].SessionKey == s.SessionKey {
			return SessionInfo{}, false
		}
	}
	c.Sessions = append(c.Sessions, s)
	return s, true
}

func (c *Config) SetSessionOrgID(sessionKey, orgID string) {
	c.RwMutx.Lock()
	defer c.RwMutx.Unlock()
	for i := range c.Sessions {
		if c.Sessions[i].SessionKey == sessionKey {
			logger.Info(fmt.Sprintf("Setting OrgID for session %s to %s", sessionKey, orgID))
			c.Sessions[i].OrgID = orgID
			return
		}
	}
}

// SetSessionDisplayName labels a session with the account email (or any human
// label). Used to auto-populate the UI name from /api/account.
func (c *Config) SetSessionDisplayName(sessionKey, name string) {
	c.RwMutx.Lock()
	defer c.RwMutx.Unlock()
	for i := range c.Sessions {
		if c.Sessions[i].SessionKey == sessionKey {
			c.Sessions[i].DisplayName = name
			return
		}
	}
}
// Session cooldown: after a failed request an account is parked for a short
// while so later requests skip it and don't waste time re-hitting a session
// that just failed (e.g. rate-limited).
// ponytail: fixed 30s TTL, no backoff ladder or per-model keys — add if a
// session needs to cool longer per model tier.
const sessionCooldownTTL = 30 * time.Second

var (
	cooldownMu sync.Mutex
	cooldownAt = map[string]time.Time{}
)

func CooldownSession(sessionKey string) {
	cooldownMu.Lock()
	cooldownAt[sessionKey] = time.Now().Add(sessionCooldownTTL)
	cooldownMu.Unlock()
}

func SessionCooling(sessionKey string) bool {
	cooldownMu.Lock()
	defer cooldownMu.Unlock()
	until, ok := cooldownAt[sessionKey]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		delete(cooldownAt, sessionKey)
		return false
	}
	return true
}

// ClearCooldown forgets every cooldown (tests only).
func ClearCooldown() {
	cooldownMu.Lock()
	cooldownAt = map[string]time.Time{}
	cooldownMu.Unlock()
}

func (sr *SessionRagen) NextIndex() int {
	sr.Mutex.Lock()
	defer sr.Mutex.Unlock()

	index := sr.Index
	sr.Index = (index + 1) % len(ConfigInstance.Sessions)
	return index
}

// 检查配置文件是否存在
func configFileExists() (bool, string) {
	execDir := filepath.Dir(os.Args[0])
	workDir, _ := os.Getwd()
	if execDir == "" && workDir == "" {
		logger.Error("Failed to get executable directory")
		return false, ""
	}

	var err error
	exeConfigPath := filepath.Join(execDir, "config.yaml")
	_, err = os.Stat(exeConfigPath)
	if !os.IsNotExist(err) {
		return true, exeConfigPath
	}

	workConfigPath := filepath.Join(workDir, "config.yaml")
	_, err = os.Stat(workConfigPath)
	if !os.IsNotExist(err) {
		return true, workConfigPath
	}

	return false, ""
}

// 从YAML文件加载配置
func loadConfigFromYAML(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %v", err)
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// 设置读写锁（不从YAML加载）
	config.RwMutx = sync.RWMutex{}
	for i := range config.Sessions {
		config.Sessions[i].mu = &sync.Mutex{}
	}

	// 如果地址为空，使用默认值
	if config.Address == "" {
		config.Address = "0.0.0.0:8080"
	}

	return &config, nil
}

// CookieFile holds one entry of a browser-export cookie.json (EditThisCookie /
// Cookie-Editor format).
type CookieFile struct {
	Domain         string  `json:"domain"`
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	ExpirationDate float64 `json:"expirationDate"`
	HostOnly       bool    `json:"hostOnly"`
	SessionCookie  bool    `json:"session"`
	HttpOnly       bool    `json:"httpOnly"`
	Secure         bool    `json:"secure"`
	Path           string  `json:"path"`
}

// buildSessionFromCookies converts a browser cookie array (EditThisCookie /
// Cookie-Editor format) into a SessionInfo: sessionKey from the sessionKey
// cookie, DeviceID from anthropic-device-id, org from lastActiveOrg, and the
// full claude.ai cookie list as the Cookie header so upstream requests carry
// the same browser state. Missing sessionKey → not ok.
func buildSessionFromCookies(displayName string, cookies []CookieFile) (SessionInfo, bool) {
	var sessionKey, deviceID, orgID string
	var pairs []string
	for _, c := range cookies {
		if !strings.Contains(c.Domain, "claude.ai") {
			continue
		}
		switch c.Name {
		case "sessionKey", "sessionKeyV3":
			if sessionKey == "" && c.Value != "" {
				sessionKey = c.Value
			}
		case "anthropic-device-id":
			deviceID = c.Value
		case "lastActiveOrg":
			orgID = c.Value
		}
		pairs = append(pairs, c.Name+"="+c.Value)
	}
	if sessionKey == "" {
		return SessionInfo{}, false
	}
	s := NewSessionInfoWithCookie(sessionKey, orgID, displayName, strings.Join(pairs, "; "), deviceID)
	s.CookieJSON = cookies
	return *s, true
}

// LoadSessionFromCookie reads a single cookie file (every ".claude.ai" cookie)
// and returns one SessionInfo. Kept for compatibility with legacy cookie.json.
func LoadSessionFromCookie(cookiePath string) (SessionInfo, bool) {
	data, err := os.ReadFile(cookiePath)
	if err != nil {
		return SessionInfo{}, false
	}
	var cookies []CookieFile
	if err := json.Unmarshal(data, &cookies); err != nil {
		logger.Error(fmt.Sprintf("Failed to parse cookie file %s: %v", cookiePath, err))
		return SessionInfo{}, false
	}
	return buildSessionFromCookies("", cookies)
}

// CookieAccount is one cookie-account stored in accounts.json: an optional
// display name plus the exported browser cookie array.
type CookieAccount struct {
	Name     string       `json:"name,omitempty"`
	Disabled bool         `json:"disabled,omitempty"`
	Cookies  []CookieFile `json:"cookies"`
}

// LoadCookieAccounts reads accounts.json (array of CookieAccount) and the
// legacy cookie.json (single account), building the full cookie pool. Returns
// the accounts plus the legacy session if present.
func LoadCookieAccounts(accountsPath string) ([]SessionInfo, bool) {
	// Legacy single cookie.json.
	var out []SessionInfo
	var ok bool
	if s, ok2 := LoadSessionFromCookie("cookie.json"); ok2 {
		out = append(out, s)
		ok = true
	}
	// accounts.json: array of named accounts.
	data, err := os.ReadFile(accountsPath)
	if err != nil {
		return out, ok
	}
	var accounts []CookieAccount
	if err := json.Unmarshal(data, &accounts); err != nil {
		logger.Error(fmt.Sprintf("Failed to parse %s: %v", accountsPath, err))
		return out, ok
	}
	for _, a := range accounts {
		if len(a.Cookies) == 0 {
			continue
		}
		if s, ok2 := buildSessionFromCookies(a.Name, a.Cookies); ok2 {
			s.Disabled = a.Disabled
			out = append(out, s)
			ok = true
		}
	}
	return out, ok
}

// 从环境变量加载配置
func loadConfigFromEnv() *Config {
	maxChatHistoryLength, err := strconv.Atoi(os.Getenv("MAX_CHAT_HISTORY_LENGTH"))
	if err != nil {
		maxChatHistoryLength = 10000 // 默认值
	}
	// Cookie accounts are now the source of truth. Load accounts.json (named
	// accounts) + legacy cookie.json; fall back to the SESSIONS env var only
	// when no cookie account is configured.
	sessions, haveCookie := LoadCookieAccounts("accounts.json")
	if !haveCookie {
		sessions = parseSessionEnv(os.Getenv("SESSIONS"))
	}
	config := &Config{
		// 解析 SESSIONS 环境变量
		Sessions: sessions,
		// 设置服务地址，默认为 "0.0.0.0:8080"
		Address: os.Getenv("ADDRESS"),

		// 设置 API 认证密钥
		APIKey: os.Getenv("APIKEY"),
		// 设置代理地址
		Proxy: os.Getenv("PROXY"),
		// 自动删除聊天
		ChatDelete: os.Getenv("CHAT_DELETE") != "false",
		// 设置最大聊天历史长度
		MaxChatHistoryLength: maxChatHistoryLength,
		// 设置是否使用角色前缀
		NoRolePrefix: os.Getenv("NO_ROLE_PREFIX") == "true",
		// 设置是否使用提示词禁用artifacts
		PromptDisableArtifacts: os.Getenv("PROMPT_DISABLE_ARTIFACTS") == "true",
		// 设置是否启用镜像API
		EnableMirrorApi: os.Getenv("ENABLE_MIRROR_API") == "true",
		// 设置镜像API前缀
		MirrorApiPrefix: os.Getenv("MIRROR_API_PREFIX"),
		// 设置是否启用网关（本地模拟 claude.ai 网页）
		EnableGateway: os.Getenv("ENABLE_GATEWAY") == "true",
		// Bật/tắt hỗ trợ tool (mặc định bật)
		EnableTools: os.Getenv("ENABLE_TOOLS") != "false",
		// 网关优先使用的 sessionKey（可选）
		GatewayKey: os.Getenv("GATEWAY_KEY"),
		// 设置读写锁
		RwMutx: sync.RWMutex{},
	}
	for i := range config.Sessions {
		config.Sessions[i].mu = &sync.Mutex{}
	}

	// 如果地址为空，使用默认值
	if config.Address == "" {
		config.Address = "0.0.0.0:8080"
	}
	return config
}

// 加载配置
func LoadConfig() *Config {
	// 检查配置文件是否存在
	exists, configPath := configFileExists()
	if exists {
		logger.Info(fmt.Sprintf("Found config file at %s", configPath))
		config, err := loadConfigFromYAML(configPath)
		if err == nil {
			logger.Info("Successfully loaded configuration from YAML file")
			return config
		}
		logger.Error(fmt.Sprintf("Failed to load config from YAML: %v, falling back to environment variables", err))
	}

	// 如果配置文件不存在或加载失败，从环境变量加载
	logger.Info("Loading configuration from environment variables")
	return loadConfigFromEnv()
}

var ConfigInstance *Config
var Sr *SessionRagen

func init() {
	rand.Seed(time.Now().UnixNano())
	// 加载环境变量
	_ = godotenv.Load()
	Sr = &SessionRagen{
		Index: 0,
		Mutex: sync.Mutex{},
	}
	ConfigInstance = LoadConfig()
	logger.Info("Loaded config:")
	for i := range ConfigInstance.Sessions {
		logger.Info(fmt.Sprintf("Session: %s, OrgID: %s", ConfigInstance.Sessions[i].SessionKey, ConfigInstance.Sessions[i].OrgID))
	}
	logger.Info(fmt.Sprintf("Address: %s", ConfigInstance.Address))
	logger.Info(fmt.Sprintf("APIKey: %s", ConfigInstance.APIKey))
	logger.Info(fmt.Sprintf("Proxy: %s", ConfigInstance.Proxy))
	logger.Info(fmt.Sprintf("ChatDelete: %t", ConfigInstance.ChatDelete))
	logger.Info(fmt.Sprintf("MaxChatHistoryLength: %d", ConfigInstance.MaxChatHistoryLength))
	logger.Info(fmt.Sprintf("NoRolePrefix: %t", ConfigInstance.NoRolePrefix))
	logger.Info(fmt.Sprintf("PromptDisableArtifacts: %t", ConfigInstance.PromptDisableArtifacts))
	logger.Info(fmt.Sprintf("EnableMirrorApi: %t", ConfigInstance.EnableMirrorApi))
	logger.Info(fmt.Sprintf("MirrorApiPrefix: %s", ConfigInstance.MirrorApiPrefix))
	logger.Info(fmt.Sprintf("EnableGateway: %t", ConfigInstance.EnableGateway))
	logger.Info(fmt.Sprintf("EnableTools: %t", ConfigInstance.EnableTools))
}
