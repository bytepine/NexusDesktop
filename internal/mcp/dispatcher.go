// Copyright byteyang. All Rights Reserved.

// Package mcp 实现 MCP 协议的 HTTP 服务器与 JSON-RPC 分发器。
// Dispatcher 行为与 nexus-vscode 的 NexusMcpDispatcher 对齐：
//   - initialize/initialized/ping/tools/list/tools/call 状态机
//   - initialize 时异步预热 UE 连接（INITIALIZE_WARMUP_MS 超时后先返回 prefix）
//   - tools/list 合并代理本地工具 + UE 远端工具
//   - tools/call 支持 arguments.targetPort 一次性路由
package mcp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bytepine/NexusDesktop/internal/log"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

// JSON-RPC 2.0 错误码
const (
	errParseError     = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternalError  = -32603
)

const (
	protocolVersion   = "2025-06-18"
	serverName        = "Nexus-Desktop"
	initializeWarmupMs = 2000 * time.Millisecond
)

type sessionState int

const (
	stateWaitingForInitialize sessionState = iota
	stateWaitingForInitialized
	stateRunning
)

// Dispatcher 处理单个 MCP 会话的 JSON-RPC 消息分发。
type Dispatcher struct {
	state   sessionState
	manager *unreal.Manager
	version string
	// onSessionReady 在 initialized 完成后调用（用于向 SSE 客户端推送 tools/list_changed）
	onSessionReady func()
}

// NewDispatcher 创建分发器实例。version 为程序版本号。
func NewDispatcher(mgr *unreal.Manager, onReady func(), version string) *Dispatcher {
	return &Dispatcher{
		state:          stateWaitingForInitialize,
		manager:        mgr,
		version:        version,
		onSessionReady: onReady,
	}
}

// IsWaitingForInitialize 返回会话是否仍处于初始化前状态（用于陈旧会话淘汰）。
func (d *Dispatcher) IsWaitingForInitialize() bool {
	return d.state == stateWaitingForInitialize
}

// Dispatch 处理一条 JSON-RPC 消息字符串，返回响应 JSON 或空串（通知类无需回复）。
func (d *Dispatcher) Dispatch(body string) (string, error) {
	var msg map[string]interface{}
	if err := json.Unmarshal([]byte(body), &msg); err != nil {
		return makeError(nil, errParseError, "Parse error"), nil
	}
	if msg["jsonrpc"] != "2.0" {
		return makeError(nil, errInvalidRequest, "Invalid JSON-RPC version"), nil
	}
	method, _ := msg["method"].(string)
	if method == "" {
		return makeError(nil, errInvalidRequest, "Missing method"), nil
	}
	id := msg["id"]
	params, _ := msg["params"].(map[string]interface{})

	log.Debugf("MCP dispatch method=%s id=%v", method, id)

	switch method {
	case "initialize":
		return d.handleInitialize(id, params)
	case "notifications/initialized":
		d.handleInitialized()
		return "", nil
	case "ping":
		return makeResult(id, map[string]interface{}{}), nil
	case "tools/list":
		if d.state != stateRunning {
			return makeError(id, errInvalidRequest, "Session not initialized"), nil
		}
		return d.handleToolsList(id)
	case "tools/call":
		if d.state != stateRunning {
			return makeError(id, errInvalidRequest, "Session not initialized"), nil
		}
		return d.handleToolsCall(id, params)
	default:
		if id != nil {
			return makeError(id, errMethodNotFound, "Method not found: "+method), nil
		}
		return "", nil
	}
}

// handleInitialize 处理 initialize 握手，并触发 UE 预热（带超时）。
func (d *Dispatcher) handleInitialize(id interface{}, params map[string]interface{}) (string, error) {
	d.state = stateWaitingForInitialize

	clientVersion, _ := params["protocolVersion"].(string)
	if clientVersion == "" {
		clientVersion = protocolVersion
	}

	// 取上次缓存 instructions
	upstream := d.manager.GetUpstreamInstructions()

	// 预热：最多等 INITIALIZE_WARMUP_MS，超时先返回 prefix，后台继续
	warmupDone := make(chan struct{}, 1)
	go func() {
		defer func() { warmupDone <- struct{}{} }()
		d.manager.MaintainConnection()
		if d.manager.IsWsOpen() {
			_ = d.manager.FetchProxyConfig()
			upstream = d.manager.FetchUpstreamInstructions()
		}
	}()
	select {
	case <-warmupDone:
	case <-time.After(initializeWarmupMs):
		// 超时先返回，warmup goroutine 后台继续
	}

	proxyCfg := d.manager.GetProxyConfig()
	connNote := "(Connected via NexusDesktop.)"
	if !d.manager.IsWsOpen() {
		connNote = "(UE not connected — call list_unreal_instances + connect_unreal_instance when needed.)"
	}
	instructions := proxyCfg.InitializePrefix + "\n" + connNote
	if upstream != "" {
		instructions += "\n\n--- Upstream (Unreal) ---\n" + upstream
	}

	result := map[string]interface{}{
		"protocolVersion": clientVersion,
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{"listChanged": true},
		},
		"serverInfo": map[string]interface{}{
			"name":    serverName,
			"version": d.version,
		},
		"instructions":  instructions,
		"cache_control": map[string]interface{}{"type": "ephemeral"},
	}
	d.state = stateWaitingForInitialized
	return makeResult(id, result), nil
}

// handleInitialized 收到 notifications/initialized 后进入 Running 状态。
func (d *Dispatcher) handleInitialized() {
	if d.state != stateWaitingForInitialized {
		return
	}
	d.state = stateRunning
	go func() {
		_, _ = d.manager.FetchToolsList()
		if d.onSessionReady != nil {
			d.onSessionReady()
		}
	}()
}

// handleToolsList 返回代理本地工具 + UE 远端工具的合并列表。
func (d *Dispatcher) handleToolsList(id interface{}) (string, error) {
	proxyCfg := d.manager.GetProxyConfig()
	var tools []interface{}
	for _, t := range proxyCfg.ConnectionTools {
		tools = append(tools, map[string]interface{}{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": json.RawMessage(t.InputSchema),
		})
	}
	remote, _ := d.manager.FetchToolsList()
	tools = append(tools, remote...)
	return makeResult(id, map[string]interface{}{"tools": tools}), nil
}

// handleToolsCall 路由 tools/call：代理本地工具直接处理，其余转发 UE。
func (d *Dispatcher) handleToolsCall(id interface{}, params map[string]interface{}) (string, error) {
	if params == nil {
		return makeError(id, errInvalidParams, "Missing params"), nil
	}
	toolName, _ := params["name"].(string)
	if toolName == "" {
		return makeError(id, errInvalidParams, "Missing tool name"), nil
	}
	log.Debugf("tools/call tool=%s", toolName)

	proxyCfg := d.manager.GetProxyConfig()

	// 代理自有工具
	if proxyCfg.IsLocalTool(toolName) {
		switch toolName {
		case "list_unreal_instances":
			return d.handleListInstances(id)
		case "connect_unreal_instance":
			args, _ := params["arguments"].(map[string]interface{})
			return d.handleConnect(id, args)
		}
	}

	// 解析可选 targetPort（一次性路由不改变长连接绑定）
	args, _ := params["arguments"].(map[string]interface{})
	forwardParams := cloneMap(params)
	targetPort := -1
	if args != nil {
		if tp, ok := args["targetPort"].(float64); ok && tp >= 1024 {
			targetPort = int(tp)
			newArgs := cloneMap(args)
			delete(newArgs, "targetPort")
			forwardParams["arguments"] = newArgs
		}
	}

	// 转发到 UE
	var outcome unreal.WsRequestResult
	if targetPort > 0 {
		outcome = d.manager.ForwardToolCallToPort(targetPort, forwardParams)
	} else {
		if !d.manager.EnsureLongConnection() {
			return makeError(id, errInternalError, proxyCfg.ErrorMessages.NotConnected), nil
		}
		outcome = d.manager.ForwardToolCall(forwardParams)
		if outcome.Status == "disconnected" {
			if d.manager.EnsureLongConnection() {
				outcome = d.manager.ForwardToolCall(forwardParams)
			}
		}
	}

	switch outcome.Status {
	case "disconnected":
		d.manager.EnqueueProxyFeedback(unreal.ProxyFeedbackEvent{
			Category:  unreal.ProxyFeedbackDisconnect,
			Tool:      toolName,
			ErrorText: proxyCfg.ErrorMessages.NotConnected,
		})
		d.manager.FlushProxyFeedback()
		return makeErrorWithData(id, errInternalError, proxyCfg.ErrorMessages.NotConnected,
			map[string]interface{}{"errorKind": "proxy_not_connected"}), nil
	case "timeout":
		sec := int(unreal.ToolsCallTimeout().Seconds())
		msg := fmt.Sprintf("UE request timed out after %ds. %s", sec, proxyCfg.ErrorMessages.TimeoutHint)
		d.manager.EnqueueProxyFeedback(unreal.ProxyFeedbackEvent{
			Category:  unreal.ProxyFeedbackTimeout,
			Tool:      toolName,
			ErrorText: msg,
		})
		d.manager.FlushProxyFeedback()
		return makeErrorWithData(id, errInternalError, msg,
			map[string]interface{}{"errorKind": "proxy_timeout"}), nil
	}

	resp := outcome.Response
	if result, ok := resp["result"]; ok {
		return makeResult(id, result), nil
	}
	if errObj, ok := resp["error"].(map[string]interface{}); ok {
		code := errInternalError
		if c, ok := errObj["code"].(float64); ok {
			code = int(c)
		}
		msg, _ := errObj["message"].(string)
		return makeError(id, code, msg), nil
	}
	return makeError(id, errInternalError, "Invalid response from UE instance"), nil
}

// handleListInstances 列出所有已发现的 UE 实例。
func (d *Dispatcher) handleListInstances(id interface{}) (string, error) {
	instances := d.manager.DiscoverInstances()
	wsOpen := d.manager.IsWsOpen()
	connPort := d.manager.ConnectedPort
	var arr []interface{}
	for _, info := range instances {
		entry := map[string]interface{}{
			"port":          info.Port,
			"projectName":   info.ProjectName,
			"engineVersion": info.EngineVersion,
			"connected":     info.Port == connPort && wsOpen,
		}
		if info.NetRole != "" {
			entry["netRole"] = info.NetRole
		}
		arr = append(arr, entry)
	}
	if arr == nil {
		arr = []interface{}{}
	}
	text, _ := json.MarshalIndent(arr, "", "  ")
	return makeResult(id, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{"type": "text", "text": string(text)}},
		"isError": false,
	}), nil
}

// handleConnect 连接到指定端口的 UE 实例。
func (d *Dispatcher) handleConnect(id interface{}, args map[string]interface{}) (string, error) {
	port := -1
	if args != nil {
		if p, ok := args["port"].(float64); ok {
			port = int(p)
		}
	}
	if port < 1024 {
		return makeError(id, errInvalidParams, fmt.Sprintf("Invalid port: %d", port)), nil
	}
	success := d.manager.ConnectTo(port, true)
	if success {
		_, _ = d.manager.FetchToolsList()
		if d.onSessionReady != nil {
			go d.onSessionReady()
		}
		// 连上后尝试补发断连期间积压的代理层失败事件。
		d.manager.FlushProxyFeedback()
	} else {
		d.manager.EnqueueProxyFeedback(unreal.ProxyFeedbackEvent{
			Category:  unreal.ProxyFeedbackConnectFail,
			ErrorText: fmt.Sprintf("连接失败：端口 %d 无响应", port),
		})
	}
	var msg string
	if success {
		msg = fmt.Sprintf("已连接到 UE 实例 (端口 %d)", port)
	} else {
		msg = fmt.Sprintf("连接失败：端口 %d 无响应", port)
	}
	return makeResult(id, map[string]interface{}{
		"content": []interface{}{map[string]interface{}{"type": "text", "text": msg}},
		"isError": !success,
	}), nil
}

// ------------------------------------------------------------
// JSON-RPC 工具函数
// ------------------------------------------------------------

func makeResult(id interface{}, result interface{}) string {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	b, _ := json.Marshal(msg)
	return string(b)
}

func makeError(id interface{}, code int, message string) string {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	b, _ := json.Marshal(msg)
	return string(b)
}

func makeErrorWithData(id interface{}, code int, message string, data map[string]interface{}) string {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"data":    data,
		},
	}
	b, _ := json.Marshal(msg)
	return string(b)
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
