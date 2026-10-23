// Copyright byteyang. All Rights Reserved.

package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytepine/NexusDesktop/internal/config"
	"github.com/bytepine/NexusDesktop/internal/log"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

const (
	mcpSessionHeader = "Mcp-Session-Id"
	maxSessions      = 50
	sessionIdleTime  = 30 * time.Minute
	maxSSEClients    = 32
	sseKeepaliveMs   = 20_000 * time.Millisecond
	maxBodyBytes     = 1024 * 1024
)

// Server 是 NexusDesktop 的 MCP HTTP 服务器。
//
//	POST  /stream  — Streamable HTTP，per-session 会话隔离（Mcp-Session-Id）
//	GET   /stream  — SSE 通知流（Streamable HTTP 规范）
//	GET   /sse     — SSE 通知流（旧版 MCP 客户端兼容）
//	DELETE /stream — 客户端主动结束会话
//
// 不提供 CORS 预检：带 Origin 的请求一律拒绝（非浏览器客户端）。
type Server struct {
	manager    *unreal.Manager
	version    string
	httpServer *http.Server

	mu         sync.Mutex
	sessions   map[string]*httpSession
	sseClients []*sseClient

	Port           int
	proxyToken     string
	requestCounter atomic.Uint64
}

// 会话状态不代表是否陈旧；初始化、工具调用和 SSE 均持有 active，避免被容量回收误删。
type httpSession struct {
	dispatcher *Dispatcher
	lastUsed   time.Time
	active     int
}

type sseClient struct {
	w       http.ResponseWriter
	flusher http.Flusher
	done    chan struct{}
}

// NewServer 创建 MCP HTTP 服务器实例。
func NewServer(mgr *unreal.Manager, version, proxyToken string) *Server {
	return &Server{
		manager:    mgr,
		version:    version,
		proxyToken: proxyToken,
		sessions:   make(map[string]*httpSession),
	}
}

// Start 在指定端口启动 HTTP 服务器（阻塞直到绑定成功或失败）。
// 端口占用时自动顺延，最多尝试 100 个。
func (s *Server) Start(preferredPort int, bindHost string) (int, error) {
	if bindHost == "" {
		bindHost = unreal.LoopbackHost
	}
	port, err := findAvailablePort(preferredPort, bindHost)
	if err != nil {
		return 0, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", s.handleStream)
	mux.HandleFunc("/sse", s.handleSSEEndpoint)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		logHTTPRejection(r, http.StatusNotFound, "unsupported_endpoint")
		http.NotFound(w, r)
	})

	srv := &http.Server{
		Addr:              fmt.Sprintf("%s:%d", bindHost, port),
		Handler:           http.HandlerFunc(s.guard(mux)),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	s.httpServer = srv

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return 0, err
	}
	s.Port = port
	log.Infof("MCP 服务器已就绪：http://%s:%d/stream bind=%s", unreal.McpDisplayHost(bindHost != unreal.LoopbackHost), port, bindHost)

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Errorf("MCP 服务器意外退出: %v", err)
		}
	}()
	return port, nil
}

// Stop 优雅关闭服务器。
func (s *Server) Stop() {
	if s.httpServer == nil {
		return
	}
	s.mu.Lock()
	log.Infof("MCP 服务器停止 port=%d sessions=%d sse=%d；现有会话将失效", s.Port, len(s.sessions), len(s.sseClients))
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Warnf("MCP 服务器关闭未完成: %v", err)
	}
	s.httpServer = nil
	s.Port = 0

	s.mu.Lock()
	for _, c := range s.sseClients {
		close(c.done)
	}
	s.sseClients = nil
	s.sessions = make(map[string]*httpSession)
	s.mu.Unlock()
}

// IsRunning 返回服务器是否正在运行。
func (s *Server) IsRunning() bool {
	return s.httpServer != nil
}

// SendToolsChangedNotification 向所有活跃 SSE 客户端推送 tools/list_changed。
func (s *Server) SendToolsChangedNotification() {
	const payload = `data: {"jsonrpc":"2.0","method":"notifications/tools/list_changed"}` + "\n\n"
	s.mu.Lock()
	clients := append([]*sseClient{}, s.sseClients...)
	s.mu.Unlock()

	var dead []*sseClient
	for _, c := range clients {
		select {
		case <-c.done:
			dead = append(dead, c)
		default:
			_, err := fmt.Fprint(c.w, payload)
			if err != nil {
				log.Warnf("MCP SSE 通知发送失败: %v", err)
				dead = append(dead, c)
				continue
			}
			c.flusher.Flush()
		}
	}
	if len(dead) > 0 {
		s.mu.Lock()
		alive := s.sseClients[:0]
		for _, c := range s.sseClients {
			isDead := false
			for _, d := range dead {
				if c == d {
					isDead = true
					break
				}
			}
			if !isDead {
				alive = append(alive, c)
			}
		}
		s.sseClients = alive
		s.mu.Unlock()
	}
}

func (s *Server) guard(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			logHTTPRejection(r, http.StatusForbidden, "browser_origin")
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Browser Origin is not allowed"})
			return
		}
		auth := r.Header.Get("Authorization")
		const prefix = "Bearer "
		presented := ""
		if len(auth) >= len(prefix) && strings.EqualFold(auth[:len(prefix)], prefix) {
			presented = strings.TrimSpace(auth[len(prefix):])
		}
		cfg := config.Get()
		if cfg.RequireAuth && !config.TokenAccepted(presented, s.proxyToken, cfg.ExtraAuthTokens) {
			logHTTPRejection(r, http.StatusUnauthorized, "invalid_authorization")
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Missing or invalid Authorization: Bearer token"})
			return
		}
		next.ServeHTTP(w, r)
	}
}

// handleStream 处理 POST /stream（Streamable HTTP）和 GET /stream（SSE）。
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodGet:
		s.handleSSE(w, r)
	case http.MethodDelete:
		id := r.Header.Get(mcpSessionHeader)
		s.mu.Lock()
		_, exists := s.sessions[id]
		delete(s.sessions, id)
		s.mu.Unlock()
		if !exists {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Invalid or missing Mcp-Session-Id"})
			return
		}
		log.Infof("MCP 会话移除 session=%s reason=client_delete", sessionLogID(id))
		w.WriteHeader(http.StatusNoContent)
	default:
		logHTTPRejection(r, http.StatusMethodNotAllowed, "unsupported_http_method")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleSSEEndpoint 处理 GET /sse（旧版 MCP 客户端兼容入口）。
func (s *Server) handleSSEEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		logHTTPRejection(r, http.StatusMethodNotAllowed, "unsupported_http_method")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handleSSE(w, r)
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	requestID := s.requestCounter.Add(1)
	if r.ContentLength > maxBodyBytes {
		logHTTPRejection(r, http.StatusRequestEntityTooLarge, "payload_too_large")
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "payload too large"})
		return
	}
	limited := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := limited.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			logHTTPRejection(r, http.StatusRequestEntityTooLarge, "body_read_failed")
			// 超限（ContentLength 缺失时只有这里能拦）不能带着截断的 body 继续，
			// 否则客户端收到的是 JSON 解析错误而不是 413
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "payload too large"})
			return
		}
	}
	body := strings.TrimSpace(sb.String())
	if body == "" {
		logHTTPRejection(r, http.StatusBadRequest, "empty_body")
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty body"})
		return
	}

	incomingSessionID := r.Header.Get(mcpSessionHeader)

	// 检测是否为 initialize 请求
	var parsed map[string]interface{}
	isInit := false
	if err := json.Unmarshal([]byte(body), &parsed); err == nil {
		isInit = parsed["method"] == "initialize"
	} else {
		isInit = strings.Contains(body, `"initialize"`)
	}

	s.mu.Lock()
	var sessionID string
	var session *httpSession
	if isInit {
		// 仅容量不足时回收空闲会话；不按状态或创建顺序删除正在使用的会话。
		now := time.Now()
		if len(s.sessions) >= maxSessions {
			for id, old := range s.sessions {
				if old.active == 0 && now.Sub(old.lastUsed) >= sessionIdleTime {
					log.Infof("MCP 会话移除 session=%s reason=idle_timeout", sessionLogID(id))
					delete(s.sessions, id)
				}
			}
		}
		if len(s.sessions) >= maxSessions {
			s.mu.Unlock()
			logHTTPRejection(r, http.StatusServiceUnavailable, "session_limit")
			w.Header().Set("Retry-After", "1")
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "too many active MCP sessions"})
			return
		}
		sessionID = newSessionID()
		session = &httpSession{dispatcher: NewDispatcher(s.manager, func() { s.SendToolsChangedNotification() }, s.version), lastUsed: now}
		s.sessions[sessionID] = session
	} else if incomingSessionID != "" {
		if existing, ok := s.sessions[incomingSessionID]; ok {
			sessionID = incomingSessionID
			session = existing
		}
	}
	if session != nil {
		session.active++
		session.lastUsed = time.Now()
	}
	activeSessions := len(s.sessions)
	s.mu.Unlock()

	if session == nil {
		reason := "unknown_session"
		if incomingSessionID == "" {
			reason = "missing_session"
		}
		logHTTPRejection(r, http.StatusNotFound, reason)
		log.Warnf("MCP 会话查找失败 request=%d session=%s %s sessions=%d；客户端需要重新 initialize", requestID, sessionLogID(incomingSessionID), rpcLogFields(parsed), activeSessions)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Invalid or missing Mcp-Session-Id"})
		return
	}
	defer s.releaseSession(session)

	fields := fmt.Sprintf("request=%d %s", requestID, rpcLogFields(parsed))
	if isInit {
		params, _ := parsed["params"].(map[string]interface{})
		client, _ := params["clientInfo"].(map[string]interface{})
		name, _ := client["name"].(string)
		version, _ := client["version"].(string)
		protocol, _ := params["protocolVersion"].(string)
		log.Infof("MCP 新会话 session=%s previous=%s client=%.80q version=%.80q protocol=%.80q remote=%.80q sessions=%d", sessionLogID(sessionID), sessionLogID(incomingSessionID), name, version, protocol, r.RemoteAddr, activeSessions)
	} else if parsed["method"] == "tools/call" {
		log.Infof("MCP 请求开始 session=%s %s", sessionLogID(sessionID), fields)
	}
	resp, _ := session.dispatcher.Dispatch(body)
	// 只记录结果类型与错误码，不把参数、响应正文或错误正文写入日志。
	var result struct {
		Error *struct {
			Code int
			Data struct{ ErrorKind string }
		}
		Result struct{ IsError bool }
	}
	_ = json.Unmarshal([]byte(resp), &result)
	if result.Error != nil {
		log.Warnf("MCP 请求失败 session=%s %s rpc_code=%d error_kind=%.80q elapsed_ms=%d", sessionLogID(sessionID), fields, result.Error.Code, result.Error.Data.ErrorKind, time.Since(started).Milliseconds())
	} else if result.Result.IsError {
		log.Warnf("MCP 工具失败 session=%s %s isError=true elapsed_ms=%d", sessionLogID(sessionID), fields, time.Since(started).Milliseconds())
	} else if isInit || parsed["method"] == "notifications/initialized" || parsed["method"] == "tools/list" || parsed["method"] == "tools/call" {
		log.Infof("MCP 请求完成 session=%s %s elapsed_ms=%d", sessionLogID(sessionID), fields, time.Since(started).Milliseconds())
	}
	w.Header().Set(mcpSessionHeader, sessionID)
	if resp == "" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, resp); err != nil {
		log.Warnf("MCP 响应发送失败 session=%s %s: %v", sessionLogID(sessionID), fields, err)
	}
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	flusher, ok := w.(http.Flusher)
	if !ok {
		logHTTPRejection(r, http.StatusInternalServerError, "sse_not_supported")
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	if len(s.sseClients) >= maxSSEClients {
		s.mu.Unlock()
		logHTTPRejection(r, http.StatusServiceUnavailable, "sse_limit")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "too many SSE clients"})
		return
	}
	client := &sseClient{w: w, flusher: flusher, done: make(chan struct{})}
	s.sseClients = append(s.sseClients, client)
	session := s.sessions[r.Header.Get(mcpSessionHeader)]
	if session != nil {
		session.active++
		session.lastUsed = time.Now()
	}
	clientCount := len(s.sseClients)
	s.mu.Unlock()
	log.Infof("MCP SSE 已连接 session=%s remote=%.80q clients=%d", sessionLogID(r.Header.Get(mcpSessionHeader)), r.RemoteAddr, clientCount)
	defer func() {
		if session != nil {
			s.releaseSession(session)
		}
		log.Infof("MCP SSE 已断开 session=%s remote=%.80q elapsed_ms=%d", sessionLogID(r.Header.Get(mcpSessionHeader)), r.RemoteAddr, time.Since(started).Milliseconds())
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// 心跳 goroutine
	go func() {
		ticker := time.NewTicker(sseKeepaliveMs)
		defer ticker.Stop()
		for {
			select {
			case <-client.done:
				return
			case <-r.Context().Done():
				return
			case <-ticker.C:
				select {
				case <-client.done:
					return
				default:
					if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
						log.Warnf("MCP SSE 心跳发送失败 session=%s: %v", sessionLogID(r.Header.Get(mcpSessionHeader)), err)
						return
					}
					flusher.Flush()
				}
			}
		}
	}()

	// 等待客户端断开
	<-r.Context().Done()
	close(client.done)
	s.mu.Lock()
	filtered := s.sseClients[:0]
	for _, c := range s.sseClients {
		if c != client {
			filtered = append(filtered, c)
		}
	}
	s.sseClients = filtered
	s.mu.Unlock()
}

func (s *Server) releaseSession(session *httpSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session.active--
	session.lastUsed = time.Now()
}

// ------------------------------------------------------------
// 工具函数
// ------------------------------------------------------------

// 会话标识只写摘要；仍可将创建、淘汰和失败请求关联起来。
func sessionLogID(id string) string {
	if id == "" {
		return "none"
	}
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%x", sum[:6])
}

func logHTTPRejection(r *http.Request, status int, reason string) {
	log.Warnf("MCP HTTP 请求拒绝 status=%d reason=%s http_method=%.16q path=%.80q remote=%.80q session=%s", status, reason, r.Method, r.URL.EscapedPath(), r.RemoteAddr, sessionLogID(r.Header.Get(mcpSessionHeader)))
}

func rpcLogFields(msg map[string]interface{}) string {
	method, _ := msg["method"].(string)
	params, _ := msg["params"].(map[string]interface{})
	tool, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]interface{})
	capKey, fallbackKey := "Capability", "capability"
	if tool == "call_capability" {
		capKey, fallbackKey = fallbackKey, capKey
	}
	capability, _ := args[capKey].(string)
	if capability == "" {
		capability, _ = args[fallbackKey].(string)
	}
	return fmt.Sprintf("method=%.80q tool=%.80q capability=%.80q", method, tool, capability)
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	b, _ := json.Marshal(v)
	_, _ = w.Write(b)
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// findAvailablePort 从 startPort 开始顺延寻找可用端口，最多尝试 100 个。
func findAvailablePort(startPort int, bindHost string) (int, error) {
	for i := 0; i < 100; i++ {
		port := startPort + i
		if port > 65535 {
			break
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", bindHost, port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("端口 %d 及后续 100 个端口均被占用", startPort)
}
