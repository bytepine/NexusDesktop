// Copyright byteyang. All Rights Reserved.

// Package config 管理 NexusDesktop 持久化配置。
// 读写 <UserConfigDir>/NexusDesktop/config.json，字段与 nexus-vscode 的 nexusMcp.* 对齐，
// 方便用户在两套方案间切换时配置含义一致。
package config

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/bytepine/NexusDesktop/internal/log"
)

const appDirName = "NexusDesktop"

// RemoteUnreal 是显式配置的远程 UE。
type RemoteUnreal struct {
	Host      string `json:"host"`
	McpPort   int    `json:"mcpPort"`
	AuthToken string `json:"authToken"`
}

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
	Language  string `json:"language"`
	ListenLan bool   `json:"listenLan"`
	// RequireAuth 为 true 时 AI→本机 MCP 须 Bearer；缺省视为 true。
	RequireAuth bool `json:"requireAuth"`
	// ExtraAuthTokens 其他机器的 token，换行或逗号分隔。
	ExtraAuthTokens string `json:"extraAuthTokens,omitempty"`
	// RemoteUnreal 显式远程 UE（不扫网段）。
	RemoteUnreal []RemoteUnreal `json:"remoteUnreal,omitempty"`
	// ProxyToken 是 Agent → 本机 MCP HTTP 的 Bearer。
	ProxyToken string `json:"proxyToken"`
	// UpdateChannel：auto（默认，正式仅正式 / 预发布含 beta）/ stable / pre。
	UpdateChannel string `json:"updateChannel"`
}

const (
	UpdateChannelAuto   = "auto"
	UpdateChannelStable = "stable"
	UpdateChannelPre    = "pre"

	MinPort                = 1024
	MaxPort                = 65535
	MaxScanPortSpan        = 200 // 含端点：45000–45199 合法
	MinScanIntervalSeconds = 1
)

var (
	ErrInvalidPort     = errors.New("invalid port")
	ErrInvalidInterval = errors.New("invalid scan interval")
	ErrScanSpanTooWide = errors.New("scan port span too wide")
)

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
		RequireAuth:         true,
		UpdateChannel:       UpdateChannelAuto,
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
		// 配置损坏/字段类型不兼容时先备份原文件，否则调用方一旦 Save 就把用户设置整体冲掉
		_ = os.WriteFile(configPath()+".bak", data, 0o600)
		current = DefaultConfig()
		return current, err
	}
	// Go bool 零值是 false；旧 config.json 无此键时视为开启
	var probe struct {
		RequireAuth *bool `json:"requireAuth"`
	}
	if json.Unmarshal(data, &probe) == nil && probe.RequireAuth == nil {
		cfg.RequireAuth = true
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
	if !reflect.DeepEqual(old, cfg) {
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
	if c.HTTPPort < MinPort || c.HTTPPort > MaxPort {
		c.HTTPPort = 6700
	}
	if c.ScanPortStart < MinPort || c.ScanPortStart > MaxPort {
		c.ScanPortStart = 45000
	}
	if c.ScanPortEnd < MinPort || c.ScanPortEnd > MaxPort {
		c.ScanPortEnd = 45100
	}
	if c.ScanIntervalSeconds < MinScanIntervalSeconds {
		c.ScanIntervalSeconds = 5
	}
	lo, hi := c.ScanPortStart, c.ScanPortEnd
	if lo > hi {
		lo, hi = hi, lo
	}
	if hi-lo+1 > MaxScanPortSpan {
		log.Warnf("UE 扫描区间 [%d, %d] 超过 %d 个端口，已截断", lo, hi, MaxScanPortSpan)
	}
	c.ScanPortStart, c.ScanPortEnd = ClampScanPorts(c.ScanPortStart, c.ScanPortEnd)
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
	switch c.UpdateChannel {
	case UpdateChannelAuto, UpdateChannelStable, UpdateChannelPre:
	default:
		c.UpdateChannel = UpdateChannelAuto
	}
}

func clampPort(p, fallback int) int {
	if p < MinPort || p > MaxPort {
		return fallback
	}
	return p
}

// ClampScanPorts 将起止端口收进合法范围，宽度超过 MaxScanPortSpan 时截断结束端口。
func ClampScanPorts(start, end int) (int, int) {
	start = clampPort(start, 45000)
	end = clampPort(end, 45100)
	if start > end {
		start, end = end, start
	}
	if end-start+1 > MaxScanPortSpan {
		end = start + MaxScanPortSpan - 1
		if end > MaxPort {
			end = MaxPort
			start = end - MaxScanPortSpan + 1
			if start < MinPort {
				start = MinPort
			}
		}
	}
	return start, end
}

// ParsePortField 解析设置页端口；非数字或不在 1024–65535 返回 ErrInvalidPort。
func ParsePortField(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < MinPort || n > MaxPort {
		return 0, ErrInvalidPort
	}
	return n, nil
}

// ParseScanIntervalField 解析扫描间隔秒数；非数字或 < 1 返回 ErrInvalidInterval。
func ParseScanIntervalField(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < MinScanIntervalSeconds {
		return 0, ErrInvalidInterval
	}
	return n, nil
}

// ValidateScanRange 检查扫描宽度（含端点）不超过 MaxScanPortSpan。
func ValidateScanRange(start, end int) error {
	lo, hi := start, end
	if lo > hi {
		lo, hi = hi, lo
	}
	if hi-lo+1 > MaxScanPortSpan {
		return ErrScanSpanTooWide
	}
	return nil
}

// IsPrerelease 判断版本串是否带 semver 预发布后缀（忽略 +build）。
func IsPrerelease(v string) bool {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	return strings.Contains(v, "-")
}

// ResolveUpdateChannel 把存盘值解析成 stable 或 pre。auto：当前是预发布则 pre，否则 stable。
func ResolveUpdateChannel(stored, currentVersion string) string {
	switch strings.TrimSpace(stored) {
	case UpdateChannelStable:
		return UpdateChannelStable
	case UpdateChannelPre:
		return UpdateChannelPre
	default:
		if IsPrerelease(currentVersion) {
			return UpdateChannelPre
		}
		return UpdateChannelStable
	}
}

func ensureProxyToken(c *Config) {
	c.ProxyToken = loadOrCreateMachineToken(c.ProxyToken)
}

func isValidAuthToken(s string) bool {
	n := len(s)
	if n < 32 || n > 128 {
		return false
	}
	for i := 0; i < n; i++ {
		c := s[i]
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ParseAuthTokens 按逗号/分号/空白拆出合法 token，去重保序。
func ParseAuthTokens(chunks ...string) []string {
	seen := map[string]struct{}{}
	var out []string
	split := func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}
	for _, chunk := range chunks {
		for _, p := range strings.FieldsFunc(chunk, split) {
			if !isValidAuthToken(p) {
				continue
			}
			k := strings.ToLower(p)
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}

// TokenAccepted 判断 Bearer 值（可含多个 token）是否命中本机或额外列表。
func TokenAccepted(presentedRaw, machine, extra string) bool {
	presented := ParseAuthTokens(presentedRaw)
	if len(presented) == 0 {
		return false
	}
	accepted := ParseAuthTokens(machine, extra)
	ok := false
	for _, p := range presented {
		for _, a := range accepted {
			// 常量时间比较，与 UE / VSCode / Rider 端一致；不提前 return 以免泄漏匹配位置
			if subtle.ConstantTimeCompare([]byte(p), []byte(a)) == 1 {
				ok = true
			}
		}
	}
	return ok
}

func machineAuthTokenPath() string {
	var base string
	switch runtime.GOOS {
	case "windows":
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
		}
	case "darwin":
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, "Library", "Application Support")
	default:
		base = os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			home, _ := os.UserHomeDir()
			base = filepath.Join(home, ".config")
		}
	}
	return filepath.Join(base, "NexusLink", "mcp-auth-token")
}

func readValidTokenFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	raw := strings.TrimSpace(string(data))
	if !isValidAuthToken(raw) {
		return ""
	}
	return strings.ToLower(raw)
}

// loadOrCreateMachineToken 同机 UE / Desktop / Rider / VSCode 共用一份 token。
func loadOrCreateMachineToken(seed string) string {
	path := machineAuthTokenPath()
	if existing := readValidTokenFile(path); existing != "" {
		return existing
	}
	_ = os.Remove(path)
	trimmed := strings.TrimSpace(seed)
	token := trimmed
	if !isValidAuthToken(trimmed) {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return trimmed
		}
		token = hex.EncodeToString(b)
	} else {
		token = strings.ToLower(trimmed)
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if won := readValidTokenFile(path); won != "" {
			return won
		}
		return token
	}
	_, _ = f.WriteString(token)
	_ = f.Close()
	return token
}
