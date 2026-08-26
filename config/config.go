package config

import (
	"claude2api/logger"
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
	RetryCount             int           `yaml:"retryCount"`
	NoRolePrefix           bool          `yaml:"noRolePrefix"`
	PromptDisableArtifacts bool          `yaml:"promptDisableArtifacts"`
	EnableMirrorApi        bool          `yaml:"enableMirrorApi"`
	MirrorApiPrefix        string        `yaml:"mirrorApiPrefix"`
	EnableGateway          bool          `yaml:"enableGateway"` // phục vụ giao diện claude.ai tại localhost
	GatewayKey             string        `yaml:"gatewayKey"`    // sessionKey gateway ưu tiên dùng; rỗng = tự chọn
	RwMutx                 sync.RWMutex  `yaml:"-"`             // 不从YAML加载
}

// 解析 SESSION 格式的环境变量
func parseSessionEnv(envValue string) (int, []SessionInfo) {
	if envValue == "" {
		return 0, []SessionInfo{}
	}
	var sessions []SessionInfo
	sessionPairs := strings.Split(envValue, ",")
	retryCount := len(sessionPairs) // 重试次数等于 session 数量
	for _, pair := range sessionPairs {
		if pair == "" {
			retryCount--
			continue
		}
		parts := strings.Split(pair, ":")
		orgID := ""
		if len(parts) > 1 {
			orgID = parts[1]
		}

		sessions = append(sessions, *NewSessionInfo(parts[0], orgID))
	}
	if retryCount > 5 {
		retryCount = 5 // 限制最大重试次数为 5 次
	}
	return retryCount, sessions
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

// 从环境变量加载配置
func loadConfigFromEnv() *Config {
	maxChatHistoryLength, err := strconv.Atoi(os.Getenv("MAX_CHAT_HISTORY_LENGTH"))
	if err != nil {
		maxChatHistoryLength = 10000 // 默认值
	}
	retryCount, sessions := parseSessionEnv(os.Getenv("SESSIONS"))
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
		// 设置重试次数
		RetryCount: retryCount,
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
	logger.Info(fmt.Sprintf("Max Retry count: %d", ConfigInstance.RetryCount))
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
}
