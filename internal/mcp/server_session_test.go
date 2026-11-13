// Copyright byteyang. All Rights Reserved.

package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bytepine/NexusDesktop/internal/unreal"
)

func sessionPost(s *Server, id, method string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/stream", strings.NewReader(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q}`, method)))
	r.Header.Set(mcpSessionHeader, id)
	w := httptest.NewRecorder()
	s.handlePost(w, r)
	return w
}

func TestConcurrentInitializeKeepsBothSessions(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once, unblock sync.Once
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(entered) })
		<-release
		w.WriteHeader(http.StatusNotFound)
	}))
	defer backend.Close()
	defer unblock.Do(func() { close(release) })
	endpoint, _ := url.Parse(backend.URL)
	port, _ := strconv.Atoi(endpoint.Port())
	mgr := unreal.NewManager()
	mgr.ScanPortStart, mgr.ScanPortEnd = port, port
	defer mgr.Disconnect()
	s := NewServer(mgr, "test", "")
	aDone, bDone := make(chan *httptest.ResponseRecorder, 1), make(chan *httptest.ResponseRecorder, 1)
	go func() { aDone <- sessionPost(s, "", "initialize") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first initialize did not enter warmup")
	}
	go func() { bDone <- sessionPost(s, "", "initialize") }()
	// 两次握手均进入预热，再放行；A 的 WaitingForInitialize 不是陈旧状态。
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		count := len(s.sessions)
		s.mu.Unlock()
		if count == 2 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	unblock.Do(func() { close(release) })
	a, b := <-aDone, <-bDone
	for _, result := range []*httptest.ResponseRecorder{a, b} {
		id := result.Header().Get(mcpSessionHeader)
		if result.Code != http.StatusOK || id == "" || sessionPost(s, id, "ping").Code != http.StatusOK {
			t.Fatalf("successful initialize must retain its session: status=%d id=%q", result.Code, id)
		}
	}
}

type sessionSSERecorder struct {
	*httptest.ResponseRecorder
	ready chan struct{}
	once  sync.Once
}

func (w *sessionSSERecorder) Flush() { w.once.Do(func() { close(w.ready) }) }

func TestStopClosesSSEBeforeClientDisconnect(t *testing.T) {
	s := NewServer(unreal.NewManager(), "test", "")
	s.httpServer = &http.Server{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &sessionSSERecorder{ResponseRecorder: httptest.NewRecorder(), ready: make(chan struct{})}
	r := httptest.NewRequest(http.MethodGet, "/stream", nil).WithContext(ctx)
	done := make(chan interface{}, 1)
	go func() {
		defer func() { done <- recover() }()
		s.handleSSE(w, r)
	}()
	<-w.ready
	s.Stop()
	select {
	case panicValue := <-done:
		if panicValue != nil {
			t.Fatalf("SSE stop panicked: %v", panicValue)
		}
	case <-time.After(200 * time.Millisecond):
		cancel()
		panicValue := <-done
		t.Fatalf("stop must finish SSE without waiting for the client; client disconnect panic=%v", panicValue)
	}
	// 客户端稍后断开或重复 Stop 也不能再次关闭同一 channel。
	cancel()
	s.Stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sseClients) != 0 {
		t.Fatal("stopped server retained SSE clients")
	}
}

func TestStopRejectsSSEWhileShutdownWaitsForActiveRequest(t *testing.T) {
	s := NewServer(unreal.NewManager(), "test", "")
	entered, release := make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-release
	}))
	defer backend.Close()
	defer unblock.Do(func() { close(release) })
	s.httpServer = backend.Config
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		response, err := http.Get(backend.URL)
		if err == nil {
			response.Body.Close()
		}
	}()
	<-entered
	// 现有连接的关闭信号确定 Stop 已拍下快照，但 Shutdown 仍在等待请求。
	client := &sseClient{done: make(chan struct{})}
	s.sseClients = []*sseClient{client}
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	<-client.done
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	s.handleSSE(w, httptest.NewRequest(http.MethodGet, "/stream", nil).WithContext(ctx))
	unblock.Do(func() { close(release) })
	<-requestDone
	<-stopped
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("SSE arriving during shutdown must be rejected, status=%d", w.Code)
	}
}

func TestSSEDeliversConcurrentNotificationsAndStops(t *testing.T) {
	s := NewServer(unreal.NewManager(), "test", "")
	backend := httptest.NewServer(http.HandlerFunc(s.handleSSE))
	defer backend.Close()
	s.httpServer = backend.Config
	client := backend.Client()
	client.Timeout = 3 * time.Second
	response, err := client.Get(backend.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var senders sync.WaitGroup
	for i := 0; i < 16; i++ {
		senders.Add(1)
		go func() { defer senders.Done(); s.SendToolsChangedNotification() }()
	}
	senders.Wait()
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(line, "notifications/tools/list_changed") {
		t.Fatalf("SSE notification was lost: %q (%v)", line, err)
	}
	stopped := time.Now()
	s.Stop()
	if time.Since(stopped) > time.Second {
		t.Fatal("SSE must finish before the shutdown deadline")
	}
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("SSE must end cleanly before the client closes: %v", err)
	}
}

func TestSessionCapacityProtectsActiveAndReclaimsIdle(t *testing.T) {
	port, _ := routingInstance(t, "session-test")
	mgr := unreal.NewManager()
	t.Cleanup(mgr.Disconnect)
	if !mgr.ConnectTo(port, true) {
		t.Fatal("connect protocol stub")
	}
	s := NewServer(mgr, "test", "")
	for i := 0; i < maxSessions; i++ {
		id := fmt.Sprintf("live-%d", i)
		d := NewDispatcher(mgr, nil, "test")
		d.state = stateRunning
		s.sessions[id] = &httpSession{dispatcher: d, lastUsed: time.Now()}
	}
	if result := sessionPost(s, "", "initialize"); result.Code != http.StatusServiceUnavailable || result.Header().Get("Retry-After") == "" {
		t.Fatal("full active table must reject new sessions with retryable 503")
	}
	if sessionPost(s, "live-0", "ping").Code != http.StatusOK {
		t.Fatal("new initialize evicted a live session")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &sessionSSERecorder{ResponseRecorder: httptest.NewRecorder(), ready: make(chan struct{})}
	r := httptest.NewRequest(http.MethodGet, "/stream", nil).WithContext(ctx)
	r.Header.Set(mcpSessionHeader, "live-2")
	done := make(chan struct{})
	go func() { defer close(done); s.handleSSE(w, r) }()
	<-w.ready
	s.mu.Lock()
	for _, entry := range s.sessions {
		entry.lastUsed = time.Now().Add(-sessionIdleTime)
	}
	s.sessions["live-1"].active++ // 模拟超过空闲阈值的长调用。
	s.mu.Unlock()
	if sessionPost(s, "live-0", "ping").Code != http.StatusOK || sessionPost(s, "", "initialize").Code != http.StatusOK {
		t.Fatal("requests must refresh activity; idle sessions must free capacity")
	}
	for _, id := range []string{"live-0", "live-1", "live-2"} {
		if sessionPost(s, id, "ping").Code != http.StatusOK {
			t.Fatalf("active session %s was reclaimed", id)
		}
	}
	if sessionPost(s, "live-3", "ping").Code != http.StatusNotFound {
		t.Fatal("idle session was not reclaimed")
	}
	cancel()
	<-done
	if s.sessions["live-2"].active != 0 || time.Since(s.sessions["live-2"].lastUsed) >= sessionIdleTime {
		t.Fatal("SSE close must release activity and retain the session")
	}
	r = httptest.NewRequest(http.MethodDelete, "/stream", nil)
	r.Header.Set(mcpSessionHeader, "live-0")
	deleted := httptest.NewRecorder()
	s.handleStream(deleted, r)
	if deleted.Code != http.StatusNoContent || sessionPost(s, "live-0", "ping").Code != http.StatusNotFound {
		t.Fatal("DELETE must invalidate only the requested session")
	}
	if sessionPost(s, "live-1", "ping").Code != http.StatusOK {
		t.Fatal("DELETE invalidated another session")
	}
}
