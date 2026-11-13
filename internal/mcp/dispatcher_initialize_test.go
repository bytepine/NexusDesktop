// Copyright byteyang. All Rights Reserved.

package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bytepine/NexusDesktop/internal/unreal"
	"github.com/gorilla/websocket"
)

func TestInitializeTimeoutKeepsLateInstructionsForNextHandshake(t *testing.T) {
	release := make(chan struct{})
	delivered := make(chan struct{})
	var signal sync.Once
	var unblock sync.Once
	upgrader := websocket.Upgrader{}
	var backend *httptest.Server
	backend = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			endpoint, _ := url.Parse(backend.URL)
			port, _ := strconv.Atoi(endpoint.Port())
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"server": "NexusLink", "wsPort": port})
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var request map[string]interface{}
			if conn.ReadJSON(&request) != nil {
				return
			}
			result := map[string]interface{}{}
			if request["method"] == "nexus/instructions" {
				time.AfterFunc(initializeWarmupMs+100*time.Millisecond, func() { unblock.Do(func() { close(release) }) })
				<-release
				result["instructions"] = "late upstream instructions"
			}
			if conn.WriteJSON(map[string]interface{}{"jsonrpc": "2.0", "id": request["id"], "result": result}) != nil {
				return
			}
			if request["method"] == "nexus/instructions" {
				signal.Do(func() { close(delivered) })
			}
		}
	}))
	defer backend.Close()
	defer unblock.Do(func() { close(release) })
	endpoint, _ := url.Parse(backend.URL)
	port, _ := strconv.Atoi(endpoint.Port())
	manager := unreal.NewManager()
	defer manager.Disconnect()
	manager.ScanPortStart, manager.ScanPortEnd = port, port
	if !manager.ConnectTo(port, true) {
		t.Fatal("connect mock UE")
	}
	dispatcher := NewDispatcher(manager, nil, "test")
	started := time.Now()
	response, err := dispatcher.handleInitialize(1, nil)
	if err != nil || strings.Contains(response, "late upstream instructions") {
		t.Fatalf("timed out initialize must return the initial snapshot: %s (%v)", response, err)
	}
	if time.Since(started) > initializeWarmupMs+time.Second {
		t.Fatal("initialize exceeded warmup deadline")
	}
	<-delivered
	time.Sleep(100 * time.Millisecond)
	// 再次握手须读取后台完成的缓存；race 模式还核验首次超时后的并发访问。
	response, err = dispatcher.handleInitialize(2, nil)
	if err != nil || !strings.Contains(response, "late upstream instructions") {
		t.Fatalf("next initialize must use completed instructions: %s (%v)", response, err)
	}
}
