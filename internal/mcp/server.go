// Copyright byteyang. All Rights Reserved.

package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/bytepine/NexusDesktop/internal/log"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

const (
	mcpSessionHeader = "Mcp-Session-Id"
	maxSessions      = 50
	sseKeepaliveMs   = 20_000 * time.Millisecond
)

// Server 是 NexusDesktop 的 MCP HTTP 服务器。
//
//   POST  /stream  — Streamable HTTP，per-session 会话隔离（Mcp-Session-Id）
//   GET   /stream  — SSE 通知流（Streamable HTTP 规范）
//   GET   /sse     — SSE 通知流（旧版 MCP 客户端兼容）
//   OPTIONS *      — CORS 预检
type Server struct {
	manager    *unreal.Manager
	version    string
	httpServer *http.Server

	mu           sync.Mutex
	sessions     map[string]*Dispatcher // sessionId → Dispatcher
	sessionOrder []string               // 插入序，用于 LRU 淘汰
	sseClients   []*sseClient

	Port int
}

type sseClient struct {
	w      http.ResponseWriter
	flusher http.Flusher
	done   chan struct{}
}

// NewServer 创建 MCP HTTP 服务器实例。
func NewServer(mgr *unreal.Manager, version string) *Server {
	return &Server{
		manager:  mgr,
		version:  version,
		sessions: make(map[string]*Dispatcher),
	}
}

// Start 在指定端口启动 HTTP 服务器（阻塞直到绑定成功或失败）。
// 端口占用时自动顺延，最多尝试 100 个。
func (s *Server) Start(preferredPort int) (int, error) {
	port, err := findAvailablePort(preferredPort)
	if err != nil {
		return 0, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", s.handleStream)
	mux.HandleFunc("/sse", s.handleSSEEndpoint)

	srv := &http.Server{
		Addr:    fmt.Sprintf("127.0.0.1:%d", port),
		Handler: mux,
	}
	s.httpServer = srv

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return 0, err
	}
	s.Port = port
	log.Infof("MCP 服务器已就绪：http://127.0.0.1:%d/stream", port)

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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = s.httpServer.Shutdown(ctx)
	s.httpServer = nil
	s.Port = 0

	s.mu.Lock()
	for _, c := range s.sseClients {
		close(c.done)
	}
	s.sseClients = nil
	s.sessions = make(map[string]*Dispatcher)
	s.sessionOrder = nil
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

// handleStream 处理 POST /stream（Streamable HTTP）和 GET /stream（SSE）。
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w)
	switch r.Method {
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		s.handlePost(w, r)
	case http.MethodGet:
		s.handleSSE(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleSSEEndpoint 处理 GET /sse（旧版 MCP 客户端兼容入口）。
func (s *Server) handleSSEEndpoint(w http.ResponseWriter, r *http.Request) {
	addCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	s.handleSSE(w, r)
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	body := strings.TrimSpace(sb.String())
	if body == "" {
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
	var dispatcher *Dispatcher
	if isInit {
		sessionID = newSessionID()
		log.Debugf("MCP 新会话 session=%s", sessionID[:8])
		dispatcher = NewDispatcher(s.manager, func() { s.SendToolsChangedNotification() }, s.version)
		s.sessions[sessionID] = dispatcher
		s.sessionOrder = append(s.sessionOrder, sessionID)
		// 清理其他处于 WaitingForInitialize 的陈旧会话
		for k, d := range s.sessions {
			if k != sessionID && d.IsWaitingForInitialize() {
				delete(s.sessions, k)
			}
		}
		// 超上限 LRU 淘汰
		for len(s.sessions) > maxSessions && len(s.sessionOrder) > 0 {
			oldest := s.sessionOrder[0]
			s.sessionOrder = s.sessionOrder[1:]
			if oldest != sessionID {
				delete(s.sessions, oldest)
			}
		}
	} else if incomingSessionID != "" {
		if d, ok := s.sessions[incomingSessionID]; ok {
			sessionID = incomingSessionID
			dispatcher = d
		}
	}
	s.mu.Unlock()

	if dispatcher == nil {
		log.Debugf("MCP 请求无效 session=%s", incomingSessionID)
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Invalid or missing Mcp-Session-Id"})
		return
	}

	resp, _ := dispatcher.Dispatch(body)
	w.Header().Set(mcpSessionHeader, sessionID)
	if resp == "" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, resp)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	client := &sseClient{w: w, flusher: flusher, done: make(chan struct{})}
	s.mu.Lock()
	s.sseClients = append(s.sseClients, client)
	s.mu.Unlock()

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

// ------------------------------------------------------------
// 工具函数
// ------------------------------------------------------------

func addCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Mcp-Session-Id")
	w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
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
func findAvailablePort(startPort int) (int, error) {
	for i := 0; i < 100; i++ {
		port := startPort + i
		if port > 65535 {
			break
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("端口 %d 及后续 100 个端口均被占用", startPort)
}
