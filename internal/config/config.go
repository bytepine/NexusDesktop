// Copyright byteyang. All Rights Reserved.

// Package config 管理 NexusDesktop 持久化配置。
// 读写 <UserConfigDir>/NexusDesktop/config.json，字段与 nexus-vscode 的 nexusMcp.* 对齐，
// 方便用户在两套方案间切换时配置含义一致。
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

const appDirName = "NexusDesktop"

// Config 保存全部用户配置项。
type Config struct {
	// Enabled 是中转服务器总开关；false 时不监听 MCP 端口。
	Enabled bool `json:"enabled"`
	// HTTPPort 是 AI 客户端连接的 MCP HTTP 端口（默认 6700）。
	HTTPPort int `json:"httpPort"`
	// ScanPortStart / ScanPortEnd 是 UE 实例扫描端口范围（默认 45000–45100）。
	ScanPortStart int `json:"scanPortStart"`
	ScanPortEnd   int `json:"scanPortEnd"`
	// ScanIntervalSeconds 是定时发现 UE 实例的间隔秒数（默认 5）。
	ScanIntervalSeconds int `json:"scanIntervalSeconds"`
	// WriteGate 写操作门控：off / destructive / all。
	WriteGate string `json:"writeGate"`
	// Language 是界面语言：auto（跟随系统，默认）、zh-CN、en。
	Language string `json:"language"`
	// ProxyToken 是 Agent → 本机 MCP HTTP 的 Bearer。
	ProxyToken string `json:"proxyToken"`
}

// DefaultConfig 返回内置默认配置。
func DefaultConfig() Config {
	return Config{
		Enabled:             false,
		HTTPPort:            6700,
		ScanPortStart:       45000,
		ScanPortEnd:         45100,
		ScanIntervalSeconds: 5,
		WriteGate:           "destructive",
		Language:            "auto",
	}
}

var (
	mu      sync.RWMutex
	current Config
	appDir  string

	changeCallbacks []func(Config)
)

// AppDir 返回应用数据目录（<UserConfigDir>/NexusDesktop）。
// 可在 log.Init 等处使用。
func AppDir() string {
	if appDir != "" {
		return appDir
	}
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	appDir = filepath.Join(base, appDirName)
	return appDir
}

func configPath() string {
	return filepath.Join(AppDir(), "config.json")
}

// Load 从磁盘读取配置；文件不存在时返回默认配置（不写盘）。
func Load() (Config, error) {
	mu.Lock()
	defer mu.Unlock()

	cfg := DefaultConfig()
	data, err := os.ReadFile(configPath())
	if os.IsNotExist(err) {
		ensureProxyToken(&cfg)
		current = cfg
		return cfg, nil
	}
	if err != nil {
		current = cfg
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		current = DefaultConfig()
		return current, err
	}
	sanitize(&cfg)
	ensureProxyToken(&cfg)
	current = cfg
	return cfg, nil
}

// Get 返回当前缓存配置快照（线程安全）。
func Get() Config {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Save 将 cfg 写入磁盘，并更新内存缓存，触发变更回调。
func Save(cfg Config) error {
	sanitize(&cfg)
	ensureProxyToken(&cfg)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	_ = os.MkdirAll(AppDir(), 0o755)
	if err := os.WriteFile(configPath(), data, 0o600); err != nil {
		return err
	}
	mu.Lock()
	old := current
	current = cfg
	cbs := append([]func(Config){}, changeCallbacks...)
	mu.Unlock()

	// 仅在实际值变化时触发回调
	if old != cfg {
		for _, cb := range cbs {
			cb(cfg)
		}
	}
	return nil
}

// OnChange 注册配置变更回调（每次 Save 后且值确实变化时调用）。
func OnChange(fn func(Config)) {
	mu.Lock()
	defer mu.Unlock()
	changeCallbacks = append(changeCallbacks, fn)
}

// sanitize 将不合法的字段修正为合理值。
func sanitize(c *Config) {
	if c.HTTPPort < 1024 || c.HTTPPort > 65535 {
		c.HTTPPort = 6700
	}
	if c.ScanPortStart < 1024 || c.ScanPortStart > 65535 {
		c.ScanPortStart = 45000
	}
	if c.ScanPortEnd < 1024 || c.ScanPortEnd > 65535 {
		c.ScanPortEnd = 45100
	}
	if c.ScanIntervalSeconds < 1 {
		c.ScanIntervalSeconds = 5
	}
	switch c.WriteGate {
	case "off", "destructive", "all":
	default:
		c.WriteGate = "destructive"
	}
	switch c.Language {
	case "", "auto", "zh-CN", "en":
		if c.Language == "" {
			c.Language = "auto"
		}
	default:
		c.Language = "auto"
	}
}

func ensureProxyToken(c *Config) {
	if c.ProxyToken != "" {
		return
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return
	}
	c.ProxyToken = hex.EncodeToString(b)
}
