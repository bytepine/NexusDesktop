// Copyright byteyang. All Rights Reserved.

// Package unreal 负责 UE 实例的发现、WebSocket 长连接管理与 JSON-RPC 透传。
// 行为与 nexus-vscode 的 UnrealInstanceManager 对齐：
//   - GET /status 并发端口扫描
//   - gorilla/websocket 长连接，请求串行化避免 UE GameThread 阻塞时并发积压
//   - 保活 ping（空闲 15s / 有挂起请求时 5s）
//   - tools/list 缓存 + UE 推送 tools/list_changed 失效
//   - preferredPort：用户手动指定后断线优先恢复
package unreal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytepine/NexusDesktop/internal/log"
	"github.com/gorilla/websocket"
)

const (
	toolsCallTimeoutMs     = 120_000 * time.Millisecond
	wsLightRequestTimeout  = toolsCallTimeoutMs
	wsKeepaliveIdleMs      = 15_000 * time.Millisecond
	wsKeepaliveBusyMs      = 5_000 * time.Millisecond
	scanConcurrency        = 20
	fullScanEveryNTicks    = 6
	probeMaxBytes          = 65_536
	probeTimeout           = 1 * time.Second
	wsHandshakeTimeout     = 3 * time.Second
)

// WsRequestResult 区分正常响应、断连与超时三种结果。
type WsRequestResult struct {
	Status   string                 // "ok" | "disconnected" | "timeout"
	Response map[string]interface{} // Status=="ok" 时有效
}

// ConnectionEvent 连接状态变更事件。
type ConnectionEvent struct {
	Port int // >0 表示已连接，-1 表示断开
}

// ToolsChangedEvent 工具列表变化事件（UE 端 tools/list_changed 推送）。
type ToolsChangedEvent struct{}

// Manager 管理多个 UE 实例的发现与 WebSocket 长连接。
type Manager struct {
	mu sync.Mutex

	Instances     []InstanceInfo
	ConnectedPort int
	PreferredPort int
	// manuallyDisconnected 表示用户主动断开，抑制自动重连直到用户手动连接。
	manuallyDisconnected bool

	ScanPortStart int
	ScanPortEnd   int

	connectedToolsListMode string
	ws                     *websocket.Conn
	connectionEpoch        int64
	idCounter              int64

	cachedToolsList   []interface{}
	upstreamInstructions string
	cachedProxyConfig *ProxyConfig

	pendingRequests map[int64]chan WsRequestResult

	// 请求串行链：新请求等上一个完成后再发送，避免 UE GameThread 并发排队
	requestChain chan struct{}

	keepaliveCancel context.CancelFunc

	fullScanCountdown int
	discoveryInFlight chan struct{} // 非 nil 表示有扫描在进行

	// 事件回调（非阻塞调用，调用方需在 goroutine 内注册）
	OnConnectionChanged  func(port int)
	OnToolsChanged       func()
	OnInstancesChanged   func() // 实例列表内容发生变化时触发

	// proxyFeedback 代理层转发失败（断连/超时/连接失败）的进程内缓冲，供 nexus/proxy_feedback 上报给 UE。
	proxyFeedback *proxyFeedbackBuffer
	// proxyFeedbackFlushing 并发保护：避免同一批事件被重复发送。
	proxyFeedbackFlushing int32
}

// StateSnapshot 是 Manager 当前状态的只读快照，供 UI 安全读取。
type StateSnapshot struct {
	Instances     []InstanceInfo
	ConnectedPort int
	WsOpen        bool
}

// Snapshot 在锁保护下返回当前状态快照，UI 层必须通过此方法读取，禁止直接访问字段。
func (m *Manager) Snapshot() StateSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]InstanceInfo, len(m.Instances))
	copy(cp, m.Instances)
	return StateSnapshot{
		Instances:     cp,
		ConnectedPort: m.ConnectedPort,
		WsOpen:        m.isWsOpen(),
	}
}

// NewManager 创建并返回一个 Manager，使用默认扫描范围。
func NewManager() *Manager {
	m := &Manager{
		ConnectedPort:   -1,
		PreferredPort:   -1,
		ScanPortStart:   45000,
		ScanPortEnd:     45100,
		pendingRequests: make(map[int64]chan WsRequestResult),
		requestChain:    make(chan struct{}, 1),
		proxyFeedback:   newProxyFeedbackBuffer(),
	}
	m.requestChain <- struct{}{} // 初始令牌
	return m
}

// ToolsCallTimeout 供 Dispatcher 使用的工具调用超时。
func ToolsCallTimeout() time.Duration { return toolsCallTimeoutMs }

// ------------------------------------------------------------
// 实例发现
// ------------------------------------------------------------

// MaintainConnection 定时维护入口：长连接存活时优先廉价心跳，
// 每 fullScanEveryNTicks 轮或连接断开时才做全端口扫描。
func (m *Manager) MaintainConnection() {
	m.mu.Lock()
	wsOpen := m.isWsOpen()
	port := m.ConnectedPort
	pending := len(m.pendingRequests)
	m.mu.Unlock()

	if wsOpen && port > 0 {
		if pending > 0 {
			return // 有挂起请求 = 连接正被使用，跳过探测
		}
		m.mu.Lock()
		if m.fullScanCountdown > 0 {
			m.fullScanCountdown--
			m.mu.Unlock()
			info := m.probeStatus(port)
			if info != nil {
				return // 心跳成功，省去全量扫描
			}
			m.mu.Lock()
			m.resetWsConnection(false)
			m.mu.Unlock()
		} else {
			m.mu.Unlock()
		}
	}
	m.mu.Lock()
	m.fullScanCountdown = fullScanEveryNTicks
	m.mu.Unlock()
	m.DiscoverInstances()
}

// DiscoverInstances 并发扫描端口范围，返回发现的实例列表。
func (m *Manager) DiscoverInstances() []InstanceInfo {
	// 合并并发调用
	m.mu.Lock()
	if m.discoveryInFlight != nil {
		ch := m.discoveryInFlight
		m.mu.Unlock()
		<-ch // 等待正在进行的扫描结束
		m.mu.Lock()
		instances := m.Instances
		m.mu.Unlock()
		return instances
	}
	done := make(chan struct{})
	m.discoveryInFlight = done
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.discoveryInFlight = nil
		m.mu.Unlock()
		close(done)
	}()

	found := m.scanPortsParallel()
	log.Debugf("端口扫描完成，发现 %d 个 UE 实例", len(found))

	m.mu.Lock()
	changed := !instancesEqual(m.Instances, found)
	m.Instances = found
	wsOpen := m.isWsOpen()
	connPort := m.ConnectedPort
	pending := len(m.pendingRequests)
	cb := m.OnInstancesChanged
	m.mu.Unlock()

	// 仅实例列表真正变化时才触发托盘刷新
	if changed && cb != nil {
		go cb()
	}

	// WS 已关但 connectedPort 未重置（竞态兜底）
	if connPort > 0 && !wsOpen && pending == 0 {
		m.mu.Lock()
		m.resetWsConnection(false)
		m.mu.Unlock()
	}

	// 已连接实例不在本轮结果中 → 断开
	m.mu.Lock()
	connPort = m.ConnectedPort
	m.mu.Unlock()
	if connPort > 0 {
		found2 := m.instanceList()
		exists := false
		for _, i := range found2 {
			if i.Port == connPort {
				exists = true
				break
			}
		}
		if !exists {
			m.mu.Lock()
			m.resetWsConnection(false)
			m.mu.Unlock()
		}
	}

	// 未连接时自动选择目标（用户手动断开后不自动重连）
	m.mu.Lock()
	connPort = m.ConnectedPort
	prefPort := m.PreferredPort
	manuallyDisc := m.manuallyDisconnected
	m.mu.Unlock()
	if connPort < 0 && len(found) > 0 && !manuallyDisc {
		var target *InstanceInfo
		if prefPort > 0 {
			for i := range found {
				if found[i].Port == prefPort {
					target = &found[i]
					break
				}
			}
		}
		if target == nil {
			for i := range found {
				if found[i].NetRole == "editor" || found[i].NetRole == "Editor" {
					target = &found[i]
					break
				}
			}
		}
		if target == nil {
			target = &found[0]
		}
		m.ConnectTo(target.Port, false)
	}

	return found
}

func (m *Manager) instanceList() []InstanceInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Instances
}

func (m *Manager) scanPortsParallel() []InstanceInfo {
	m.mu.Lock()
	start := m.ScanPortStart
	end := m.ScanPortEnd
	m.mu.Unlock()
	if start > end {
		start, end = end, start
	}

	var mu2 sync.Mutex
	var found []InstanceInfo

	for port := start; port <= end; port += scanConcurrency {
		var wg sync.WaitGroup
		batch := make([]int, 0, scanConcurrency)
		for p := port; p < port+scanConcurrency && p <= end; p++ {
			batch = append(batch, p)
		}
		results := make([]*InstanceInfo, len(batch))
		for i, p := range batch {
			wg.Add(1)
			go func(idx, p int) {
				defer wg.Done()
				results[idx] = m.probeStatus(p)
			}(i, p)
		}
		wg.Wait()
		mu2.Lock()
		for _, r := range results {
			if r != nil {
				found = append(found, *r)
			}
		}
		mu2.Unlock()
	}
	return found
}

func (m *Manager) probeStatus(port int) *InstanceInfo {
	client := &http.Client{Timeout: probeTimeout}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/status", port))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	lr := &io.LimitedReader{R: resp.Body, N: probeMaxBytes}
	var body map[string]interface{}
	if err := json.NewDecoder(lr).Decode(&body); err != nil {
		return nil
	}
	server, _ := body["server"].(string)
	if len(server) == 0 {
		// 兼容 server 字段不存在的旧版本：检查是否有 projectName
		if _, hasPrj := body["projectName"]; !hasPrj {
			return nil
		}
	} else {
		found := false
		for i := 0; i < len(server); i++ {
			if i+5 <= len(server) && (server[i:i+5] == "nexus" || server[i:i+5] == "Nexus") {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	wsPort, _ := body["wsPort"].(float64)
	if wsPort == 0 {
		wsPort = float64(port + 10000)
	}
	info := &InstanceInfo{
		Port:          port,
		WsPort:        int(wsPort),
		ProjectName:   stringField(body, "projectName"),
		EngineVersion: stringField(body, "engineVersion"),
		NetRole:       stringField(body, "netRole"),
		ToolsListMode: stringField(body, "toolsListMode"),
	}
	return info
}

func stringField(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// ------------------------------------------------------------
// WebSocket 连接
// ------------------------------------------------------------

// ConnectTo 通过 WebSocket 连接到指定端口的 UE 实例。
// setPreferred=true 时记录为 PreferredPort（用户手动选择）。
func (m *Manager) ConnectTo(port int, setPreferred bool) bool {
	if setPreferred {
		m.mu.Lock()
		m.PreferredPort = port
		m.manuallyDisconnected = false // 用户主动选择，恢复自动重连
		m.mu.Unlock()
	}
	m.mu.Lock()
	if m.ConnectedPort == port && m.isWsOpen() {
		m.mu.Unlock()
		return true
	}
	m.mu.Unlock()

	info := m.probeStatus(port)
	if info == nil {
		return false
	}

	m.mu.Lock()
	m.resetWsConnection(false)
	epoch := atomic.AddInt64(&m.connectionEpoch, 1)
	m.mu.Unlock()

	dialer := websocket.Dialer{HandshakeTimeout: wsHandshakeTimeout}
	conn, _, err := dialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d", info.WsPort), nil)
	if err != nil {
		log.Warnf("WS 连接失败 port=%d: %v", port, err)
		return false
	}

	m.mu.Lock()
	m.ws = conn
	m.ConnectedPort = port
	m.connectedToolsListMode = info.ToolsListMode
	m.mu.Unlock()

	log.Infof("已连接 UE 实例 port=%d wsPort=%d project=%s", port, info.WsPort, info.ProjectName)

	// 启动消息接收与保活
	go m.receiveLoop(conn, epoch)
	m.startKeepalive(conn, epoch)

	if m.OnConnectionChanged != nil {
		m.OnConnectionChanged(port)
	}

	// 异步预热配置缓存
	go func() {
		_ = m.FetchUpstreamInstructions()
		_ = m.FetchProxyConfig()
	}()

	return true
}

// Disconnect 主动断开当前连接，清除 preferredPort。
// Disconnect 主动断开连接并抑制自动重连，直到用户手动连接。
func (m *Manager) Disconnect() {
	m.mu.Lock()
	m.manuallyDisconnected = true
	m.resetWsConnection(true)
	m.mu.Unlock()
}

// IsWsOpen 返回当前 WebSocket 是否处于 OPEN 状态。
func (m *Manager) IsWsOpen() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.isWsOpen()
}

func (m *Manager) isWsOpen() bool {
	return m.ws != nil
}

// EnsureLongConnection 确保长连接可用；不可用时尝试重连或重新发现。
// 用户手动断开后不自动重连，直到下次主动连接。
func (m *Manager) EnsureLongConnection() bool {
	if m.IsWsOpen() {
		return true
	}
	m.mu.Lock()
	reconnPort := m.ConnectedPort
	if reconnPort < 0 {
		reconnPort = m.PreferredPort
	}
	manuallyDisc := m.manuallyDisconnected
	m.mu.Unlock()

	if manuallyDisc {
		return false
	}
	if reconnPort > 0 {
		return m.ConnectTo(reconnPort, false)
	}
	m.DiscoverInstances()
	return m.IsWsOpen()
}

func (m *Manager) resetWsConnection(clearPreferred bool) {
	// 调用方须持有 mu 锁
	m.releasePendingRequests()
	m.stopKeepalive()
	if m.ws != nil {
		_ = m.ws.Close()
		m.ws = nil
	}
	prev := m.ConnectedPort
	m.ConnectedPort = -1
	m.connectedToolsListMode = "starter"
	m.upstreamInstructions = ""
	m.cachedProxyConfig = nil
	if clearPreferred {
		m.PreferredPort = -1
	}
	if prev > 0 {
		cb := m.OnConnectionChanged
		if cb != nil {
			go cb(-1)
		}
	}
}

func (m *Manager) releasePendingRequests() {
	for id, ch := range m.pendingRequests {
		ch <- WsRequestResult{Status: "disconnected"}
		delete(m.pendingRequests, id)
	}
}

// receiveLoop 在独立 goroutine 中持续读取 WebSocket 消息。
func (m *Manager) receiveLoop(conn *websocket.Conn, epoch int64) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("receiveLoop panic: %v", r)
		}
	}()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			// 检查是否为当前 epoch（防旧 socket 的 close 误清新连接）
			if atomic.LoadInt64(&m.connectionEpoch) == epoch {
				log.Warnf("WS 读取错误（epoch=%d）: %v，准备重扫", epoch, err)
				m.mu.Lock()
				m.stopKeepalive()
				m.ws = nil
				prev := m.ConnectedPort
				m.ConnectedPort = -1
				m.connectedToolsListMode = "starter"
				m.upstreamInstructions = ""
				m.cachedProxyConfig = nil
				m.releasePendingRequests()
				m.mu.Unlock()
				if prev > 0 && m.OnConnectionChanged != nil {
					go m.OnConnectionChanged(-1)
				}
				go m.DiscoverInstances()
			}
			return
		}
		m.handleWsMessage(data)
	}
}

func (m *Manager) handleWsMessage(data []byte) {
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	// 匹配挂起请求响应
	if rawID, ok := msg["id"]; ok {
		var id int64
		switch v := rawID.(type) {
		case float64:
			id = int64(v)
		case string:
			fmt.Sscanf(v, "%d", &id)
		}
		m.mu.Lock()
		ch, found := m.pendingRequests[id]
		if found {
			delete(m.pendingRequests, id)
		}
		m.mu.Unlock()
		if found {
			ch <- WsRequestResult{Status: "ok", Response: msg}
		}
		return
	}
	// UE 端主动推送通知
	method, _ := msg["method"].(string)
	if method == "notifications/tools/list_changed" {
		m.mu.Lock()
		m.cachedToolsList = nil
		m.mu.Unlock()
		if m.OnToolsChanged != nil {
			go m.OnToolsChanged()
		}
	}
}

// ------------------------------------------------------------
// 保活 ping
// ------------------------------------------------------------

func (m *Manager) startKeepalive(conn *websocket.Conn, epoch int64) {
	m.mu.Lock()
	m.stopKeepalive()
	ctx, cancel := context.WithCancel(context.Background())
	m.keepaliveCancel = cancel
	m.mu.Unlock()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Errorf("keepalive panic: %v", r)
			}
		}()
		for {
			m.mu.Lock()
			busy := len(m.pendingRequests) > 0
			m.mu.Unlock()
			interval := wsKeepaliveIdleMs
			if busy {
				interval = wsKeepaliveBusyMs
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
				if atomic.LoadInt64(&m.connectionEpoch) != epoch {
					return
				}
				// 与 sendWsRequest 共用 mu，避免与 WriteJSON 并发写同一连接导致 panic
				m.mu.Lock()
				wsAlive := m.ws == conn
				var err error
				if wsAlive {
					err = conn.WriteMessage(websocket.PingMessage, nil)
				}
				m.mu.Unlock()
				if !wsAlive || err != nil {
					return
				}
			}
		}
	}()
}

func (m *Manager) stopKeepalive() {
	if m.keepaliveCancel != nil {
		m.keepaliveCancel()
		m.keepaliveCancel = nil
	}
}

// ------------------------------------------------------------
// JSON-RPC 请求
// ------------------------------------------------------------

// FetchToolsList 获取 UE 工具列表（有缓存直接返回）。
func (m *Manager) FetchToolsList() ([]interface{}, error) {
	m.mu.Lock()
	if m.cachedToolsList != nil {
		list := m.cachedToolsList
		m.mu.Unlock()
		return list, nil
	}
	m.mu.Unlock()

	outcome := m.sendWsRequest("tools/list", nil, wsLightRequestTimeout)
	if outcome.Status != "ok" {
		return nil, fmt.Errorf("tools/list: %s", outcome.Status)
	}
	result, _ := outcome.Response["result"].(map[string]interface{})
	tools, _ := result["tools"].([]interface{})
	if tools != nil {
		m.mu.Lock()
		m.cachedToolsList = tools
		m.mu.Unlock()
	}
	return tools, nil
}

// ForwardToolCall 通过长连接转发 tools/call。
func (m *Manager) ForwardToolCall(params map[string]interface{}) WsRequestResult {
	return m.sendWsRequest("tools/call", params, toolsCallTimeoutMs)
}

// EnqueueProxyFeedback 把一条代理层失败事件放入进程内缓冲，供 FlushProxyFeedback 上报。
func (m *Manager) EnqueueProxyFeedback(event ProxyFeedbackEvent) {
	m.proxyFeedback.enqueue(event)
}

// FlushProxyFeedback 异步、fire-and-forget 地尝试上报缓冲中的代理层失败事件，
// 不阻塞、不影响对 AI 的原始错误。旧版 NexusLink 未实现 nexus/proxy_feedback 时
// 静默降级：标记 unsupported 后不再重试。flush 自身失败（仍未连接/超时）时把
// 事件放回队首，等待下次连上再试，不上抛任何异常。
func (m *Manager) FlushProxyFeedback() {
	if m.proxyFeedback.isUnsupported() || !m.proxyFeedback.hasPending() {
		return
	}
	if !atomic.CompareAndSwapInt32(&m.proxyFeedbackFlushing, 0, 1) {
		return
	}
	go func() {
		defer atomic.StoreInt32(&m.proxyFeedbackFlushing, 0)
		for _, event := range m.proxyFeedback.drain() {
			if m.proxyFeedback.isUnsupported() {
				return
			}
			outcome := m.sendProxyFeedbackEvent(event)
			if outcome.Status != "ok" {
				// 反馈通道自身断连/超时：放回队首，下次连上再试，不级联报错。
				m.proxyFeedback.requeue(event)
				return
			}
			if isMethodNotFoundError(outcome.Response) {
				m.proxyFeedback.markUnsupported()
				log.Debug("UE 不支持 nexus/proxy_feedback（旧版 NexusLink），已跳过后续上报")
				return
			}
		}
	}()
}

// sendProxyFeedbackEvent 发送单条 nexus/proxy_feedback 请求，短超时避免拖慢正常连接流程。
func (m *Manager) sendProxyFeedbackEvent(event ProxyFeedbackEvent) WsRequestResult {
	params := map[string]interface{}{
		"category": string(event.Category),
		"proxy":    "desktop",
	}
	if event.Tool != "" {
		params["tool"] = event.Tool
	}
	if event.ErrorText != "" {
		params["errorText"] = event.ErrorText
	}
	if event.Note != "" {
		params["note"] = event.Note
	}
	return m.sendWsRequest("nexus/proxy_feedback", params, 3*time.Second)
}

// ForwardToolCallToPort 通过一次性 WS 连接转发到指定端口（不改动长连接）。
func (m *Manager) ForwardToolCallToPort(port int, params map[string]interface{}) WsRequestResult {
	m.mu.Lock()
	var info *InstanceInfo
	for i := range m.Instances {
		if m.Instances[i].Port == port {
			info = &m.Instances[i]
			break
		}
	}
	m.mu.Unlock()
	if info == nil {
		probed := m.probeStatus(port)
		if probed == nil {
			return WsRequestResult{Status: "disconnected"}
		}
		info = probed
	}

	dialer := websocket.Dialer{HandshakeTimeout: wsHandshakeTimeout}
	conn, _, err := dialer.Dial(fmt.Sprintf("ws://127.0.0.1:%d", info.WsPort), nil)
	if err != nil {
		return WsRequestResult{Status: "disconnected"}
	}
	defer conn.Close()

	id := atomic.AddInt64(&m.idCounter, 1)
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  params,
	}
	if err := conn.WriteJSON(req); err != nil {
		return WsRequestResult{Status: "disconnected"}
	}
	_ = conn.SetReadDeadline(time.Now().Add(toolsCallTimeoutMs))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return WsRequestResult{Status: "timeout"}
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return WsRequestResult{Status: "disconnected"}
	}
	return WsRequestResult{Status: "ok", Response: resp}
}

// FetchUpstreamInstructions 获取 UE 端 nexus/instructions（有缓存直接返回）。
func (m *Manager) FetchUpstreamInstructions() string {
	m.mu.Lock()
	if m.upstreamInstructions != "" {
		v := m.upstreamInstructions
		m.mu.Unlock()
		return v
	}
	m.mu.Unlock()

	outcome := m.sendWsRequest("nexus/instructions", nil, wsLightRequestTimeout)
	if outcome.Status != "ok" {
		return ""
	}
	result, _ := outcome.Response["result"].(map[string]interface{})
	text, _ := result["instructions"].(string)
	if text != "" {
		m.mu.Lock()
		m.upstreamInstructions = text
		m.mu.Unlock()
	}
	return text
}

// GetUpstreamInstructions 同步读取已缓存的 instructions（未连接时返回空串）。
func (m *Manager) GetUpstreamInstructions() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.upstreamInstructions
}

// FetchProxyConfig 获取 UE 端 nexus/proxy_config（有缓存直接返回）。
func (m *Manager) FetchProxyConfig() ProxyConfig {
	m.mu.Lock()
	if m.cachedProxyConfig != nil {
		cfg := *m.cachedProxyConfig
		m.mu.Unlock()
		return cfg
	}
	m.mu.Unlock()

	outcome := m.sendWsRequest("nexus/proxy_config", nil, wsLightRequestTimeout)
	if outcome.Status != "ok" {
		return DefaultProxyConfig()
	}
	result, _ := outcome.Response["result"].(map[string]interface{})
	cfg := ParseProxyConfig(result)
	m.mu.Lock()
	m.cachedProxyConfig = &cfg
	m.mu.Unlock()
	return cfg
}

// GetProxyConfig 读取代理配置（已缓存则返回 UE 配置，否则 DEFAULT）。
func (m *Manager) GetProxyConfig() ProxyConfig {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cachedProxyConfig != nil {
		return *m.cachedProxyConfig
	}
	return DefaultProxyConfig()
}

// sendWsRequest 在长连接上串行发送一条 JSON-RPC 请求并等待响应。
func (m *Manager) sendWsRequest(method string, params map[string]interface{}, timeout time.Duration) WsRequestResult {
	// 获取串行令牌
	<-m.requestChain
	defer func() { m.requestChain <- struct{}{} }()

	m.mu.Lock()
	if m.ws == nil {
		m.mu.Unlock()
		return WsRequestResult{Status: "disconnected"}
	}
	id := atomic.AddInt64(&m.idCounter, 1)
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		req["params"] = params
	}
	ch := make(chan WsRequestResult, 1)
	m.pendingRequests[id] = ch
	log.Debugf("WS → method=%s id=%d", method, id)
	err := m.ws.WriteJSON(req)
	m.mu.Unlock()

	if err != nil {
		m.mu.Lock()
		delete(m.pendingRequests, id)
		m.mu.Unlock()
		log.Warnf("WS 发送失败 method=%s: %v", method, err)
		return WsRequestResult{Status: "disconnected"}
	}

	select {
	case result := <-ch:
		log.Debugf("WS ← method=%s id=%d status=%s", method, id, result.Status)
		return result
	case <-time.After(timeout):
		m.mu.Lock()
		delete(m.pendingRequests, id)
		m.mu.Unlock()
		log.Warnf("WS 超时 method=%s id=%d", method, id)
		return WsRequestResult{Status: "timeout"}
	}
}

// Dispose 释放所有资源。
func (m *Manager) Dispose() {
	m.mu.Lock()
	m.resetWsConnection(true)
	m.mu.Unlock()
}

// instancesEqual 比较两个实例列表的端口集合是否相同（顺序无关）。
func instancesEqual(a, b []InstanceInfo) bool {
	if len(a) != len(b) {
		return false
	}
	ports := make(map[int]struct{}, len(a))
	for _, inst := range a {
		ports[inst.Port] = struct{}{}
	}
	for _, inst := range b {
		if _, ok := ports[inst.Port]; !ok {
			return false
		}
	}
	return true
}
