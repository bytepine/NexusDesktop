// Copyright byteyang. All Rights Reserved.

package mcp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/bytepine/NexusDesktop/internal/log"
	"github.com/bytepine/NexusDesktop/internal/unreal"
)

// 在独立进程中验证发布包级别的真实文件日志，避免修改其他测试的全局 logger。
func TestReleaseDiagnostics(t *testing.T) {
	if sessionLogID("abc") != "ba7816bf8f01" || sessionLogID("") != "none" {
		t.Fatal("session fingerprint must match Rider and VSCode")
	}
	const testDirEnv = "NEXUSDESKTOP_LOG_TEST_DIR"
	if dir := os.Getenv(testDirEnv); dir != "" {
		log.Level = "info"
		log.Init(dir)
		exerciseDiagnostics(t)
		return
	}
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestReleaseDiagnostics$")
	cmd.Env = append(os.Environ(), testDirEnv+"="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("diagnostics process: %v\n%s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "logs", "nexusdesktop.log"))
	if err != nil {
		t.Fatal(err)
	}
	logs := string(data)
	for _, want := range []string{
		"reason=browser_origin", "reason=missing_session", "reason=unknown_session",
		"reason=payload_too_large", "reason=empty_body", "reason=waiting_for_initialize", "reason=session_limit",
		"[INFO] MCP 新会话", `client="Test\nClient"`, `version="1.2"`, `protocol="2025-06-18"`,
		"[INFO] MCP 请求开始", `method="tools/call"`, `capability="GetExample"`,
		"[INFO] MCP 请求完成", "[WARN] MCP 请求失败", "[WARN] MCP 工具失败", "isError=true", "rpc_code=-32601", "rpc_code=-32700", "elapsed_ms=",
		"[INFO] MCP SSE 已连接", "[INFO] MCP SSE 已断开", "[INFO] MCP 服务器停止",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("missing %q in release log:\n%s", want, logs)
		}
	}
	for _, secret := range []string{"private-bearer-token", "private-session-id", "private-argument-value", "private-response-value", "private-query-value", "[DEBUG]", "Test\nClient"} {
		if strings.Contains(logs, secret) {
			t.Errorf("unexpected %q in release log", secret)
		}
	}
}

func exerciseDiagnostics(t *testing.T) {
	port, _ := routingInstance(t, "private-response-value")
	manager := unreal.NewManager()
	manager.ScanPortStart, manager.ScanPortEnd = port, port
	t.Cleanup(manager.Disconnect)
	if !manager.ConnectTo(port, true) {
		t.Fatal("connect protocol stub")
	}
	s := NewServer(manager, "test", "private-bearer-token")
	const session = "private-session-id"
	s.sessions[session] = NewDispatcher(manager, nil, "test")
	s.sessions[session].state = stateRunning
	post := func(id, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/stream?secret=private-query-value", strings.NewReader(body))
		r.Header.Set(mcpSessionHeader, id)
		r.Header.Set("Authorization", "Bearer private-bearer-token")
		w := httptest.NewRecorder()
		s.handlePost(w, r)
		if w.Code != status {
			t.Fatalf("POST status=%d want=%d body=%s", w.Code, status, w.Body.String())
		}
		return w
	}
	post("", `{"jsonrpc":"2.0","id":1,"method":"ping"}`, http.StatusNotFound)
	post("unknown-private-session-id", `{"jsonrpc":"2.0","id":2,"method":"ping"}`, http.StatusNotFound)
	post(session, "", http.StatusBadRequest)
	post(session, strings.Repeat("x", maxBodyBytes+1), http.StatusRequestEntityTooLarge)
	post(session, `{"broken":`, http.StatusOK)
	post(session, `{"jsonrpc":"2.0","id":3,"method":"unknown/rpc"}`, http.StatusOK)
	post(session, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"call_capability","arguments":{"Capability":"GetExample","Arguments":{"value":"private-argument-value"}}}}`, http.StatusOK)
	failedInstance := httptest.NewServer(http.NotFoundHandler())
	defer failedInstance.Close()
	_, failedPortText, _ := net.SplitHostPort(strings.TrimPrefix(failedInstance.URL, "http://"))
	failedPort, _ := strconv.Atoi(failedPortText)
	post(session, fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"connect_unreal_instance","arguments":{"port":%d}}}`, failedPort), http.StatusOK)

	r := httptest.NewRequest(http.MethodPost, "/stream", nil)
	r.Header.Set("Origin", "https://example.test")
	r.Header.Set("Authorization", "Bearer private-bearer-token")
	s.guard(http.HandlerFunc(s.handleStream))(httptest.NewRecorder(), r)

	// 已完成的会话超过上限，另保留一个初始化前会话以覆盖两种清理原因。
	s.sessionOrder = []string{session}
	for i := 1; i < maxSessions; i++ {
		id := fmt.Sprintf("session-%d", i)
		s.sessions[id] = &Dispatcher{state: stateRunning}
		s.sessionOrder = append(s.sessionOrder, id)
	}
	s.sessions["waiting"] = &Dispatcher{state: stateWaitingForInitialize}
	w := post("", `{"jsonrpc":"2.0","id":5,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"Test\nClient","version":"1.2"}}}`, http.StatusOK)
	newSession := w.Header().Get(mcpSessionHeader)
	if newSession == "" {
		t.Fatal("missing new session header")
	}
	post(newSession, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, http.StatusAccepted)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r = httptest.NewRequest(http.MethodGet, "/stream", nil).WithContext(ctx)
	r.Header.Set(mcpSessionHeader, newSession)
	s.handleSSE(httptest.NewRecorder(), r)
	if _, exists := s.sessions[newSession]; !exists {
		t.Fatal("closing SSE must retain the MCP session")
	}
	s.httpServer = &http.Server{}
	s.Stop()
	post(newSession, `{"jsonrpc":"2.0","id":6,"method":"ping"}`, http.StatusNotFound)
}
