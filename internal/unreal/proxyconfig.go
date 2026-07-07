// Copyright byteyang. All Rights Reserved.

package unreal

import "encoding/json"

// ConnectionTool 描述代理层自有工具（名称与 schema 由 UE nexus/proxy_config 下发）。
type ConnectionTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// ErrorMessages 是代理层错误文案，由 UE 下发，未连接时使用 defaultProxyConfig 中的 fallback。
type ErrorMessages struct {
	NotConnected string `json:"notConnected"`
	TimeoutHint  string `json:"timeoutHint"`
}

// ProxyConfig 是 nexus/proxy_config 接口返回的代理层配置。
type ProxyConfig struct {
	ProtocolVersion  string           `json:"protocolVersion"`
	MinProxyVersion  string           `json:"minProxyVersion"`
	NexusLinkVersion string           `json:"nexusLinkVersion,omitempty"`
	InitializePrefix string           `json:"initializePrefix"`
	LocalToolNames   []string         `json:"localToolNames"`
	ConnectionTools  []ConnectionTool `json:"connectionTools"`
	ErrorMessages    ErrorMessages    `json:"errorMessages"`
}

// defaultProxyConfig 是未连接 UE 时的 fallback 配置，保持 AI 客户端在离线状态仍可看到提示。
var defaultProxyConfig = ProxyConfig{
	ProtocolVersion: "2025-06-18",
	MinProxyVersion: "1.3.3",
	InitializePrefix: "NexusLink MCP Proxy for Unreal Engine. " +
		"MUST USE MCP when user mentions UE/Unreal/蓝图/Blueprint/资产/Widget/UMG/材质/PIE/Actor/GAS etc. " +
		"Do NOT guess /Game/ paths or answer from repo grep alone. " +
		"If tools/list has UE tools → call directly; if only list/connect → list → connect → search_capabilities.",
	LocalToolNames: []string{"list_unreal_instances", "connect_unreal_instance"},
	ConnectionTools: []ConnectionTool{
		{
			Name:        "list_unreal_instances",
			Description: "Discover running UE instances with NexusLink loaded.",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		{
			Name:        "connect_unreal_instance",
			Description: "Connect to a UE instance by port from list_unreal_instances.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"port":{"type":"integer"}},"required":["port"]}`),
		},
	},
	ErrorMessages: ErrorMessages{
		NotConnected: "No connected UE instance. Call connect_unreal_instance first.",
		TimeoutHint:  "Retry or narrow the query; heavy tools may need a moment.",
	},
}

// DefaultProxyConfig 返回离线 fallback 配置副本。
func DefaultProxyConfig() ProxyConfig {
	return defaultProxyConfig
}

// ParseProxyConfig 解析 nexus/proxy_config 响应；字段缺失时回退 defaultProxyConfig 对应字段。
func ParseProxyConfig(raw map[string]any) ProxyConfig {
	if raw == nil {
		return defaultProxyConfig
	}
	cfg := defaultProxyConfig

	if v, ok := raw["protocolVersion"].(string); ok && v != "" {
		cfg.ProtocolVersion = v
	}
	if v, ok := raw["minProxyVersion"].(string); ok && v != "" {
		cfg.MinProxyVersion = v
	}
	if v, ok := raw["nexusLinkVersion"].(string); ok {
		cfg.NexusLinkVersion = v
	}
	if v, ok := raw["initializePrefix"].(string); ok && v != "" {
		cfg.InitializePrefix = v
	}
	if names, ok := raw["localToolNames"].([]any); ok && len(names) > 0 {
		var ss []string
		for _, n := range names {
			if s, ok := n.(string); ok {
				ss = append(ss, s)
			}
		}
		if len(ss) > 0 {
			cfg.LocalToolNames = ss
		}
	}
	if tools, ok := raw["connectionTools"].([]any); ok && len(tools) > 0 {
		var ct []ConnectionTool
		for _, t := range tools {
			m, ok := t.(map[string]any)
			if !ok {
				continue
			}
			name, _ := m["name"].(string)
			if name == "" {
				continue
			}
			desc, _ := m["description"].(string)
			var schemaBytes json.RawMessage
			if s, ok := m["inputSchema"]; ok {
				if b, err := json.Marshal(s); err == nil {
					schemaBytes = b
				}
			}
			if schemaBytes == nil {
				schemaBytes = json.RawMessage(`{"type":"object"}`)
			}
			ct = append(ct, ConnectionTool{Name: name, Description: desc, InputSchema: schemaBytes})
		}
		if len(ct) > 0 {
			cfg.ConnectionTools = ct
		}
	}
	if errs, ok := raw["errorMessages"].(map[string]any); ok {
		if v, ok := errs["notConnected"].(string); ok && v != "" {
			cfg.ErrorMessages.NotConnected = v
		}
		if v, ok := errs["timeoutHint"].(string); ok && v != "" {
			cfg.ErrorMessages.TimeoutHint = v
		}
	}
	return cfg
}

// IsLocalTool 检查工具名是否属于代理自有工具。
func (c *ProxyConfig) IsLocalTool(name string) bool {
	for _, n := range c.LocalToolNames {
		if n == name {
			return true
		}
	}
	return false
}
