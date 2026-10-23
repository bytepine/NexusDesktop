// Copyright byteyang. All Rights Reserved.

package mcp

import (
	"context"
	"fmt"
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
