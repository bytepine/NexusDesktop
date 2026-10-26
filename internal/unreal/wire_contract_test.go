package unreal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWireAuthenticationAndWriteTimeout(t *testing.T) {
	var writes atomic.Int32
	upgrader := websocket.Upgrader{}
	port := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"server": "NexusLink", "wsPort": port})
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var req map[string]interface{}
			if conn.ReadJSON(&req) != nil {
				return
			}
			if req["method"] == "tools/call" {
				writes.Add(1)
				continue
			}
			result := map[string]interface{}{}
			if req["method"] == "auth" {
				params, _ := req["params"].(map[string]interface{})
				result["ok"] = params["token"] == "contract-test-token"
			}
			if conn.WriteJSON(map[string]interface{}{"jsonrpc": "2.0", "id": req["id"], "result": result}) != nil {
				return
			}
		}
	}))
	defer server.Close()
	port, _ = strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if authWebSocket(conn, "wrong") || !authWebSocket(conn, "contract-test-token") {
		t.Fatal("auth wire contract")
	}
	conn.Close()
	manager := NewManager()
	defer manager.Disconnect()
	if !manager.ConnectTo(port, true) {
		t.Fatal("connect")
	}
	result := manager.sendWsRequest("tools/call", map[string]interface{}{"name": "CallCapability", "arguments": map[string]interface{}{"Capability": "ManageAssetDataTable"}}, 50*time.Millisecond)
	if result.Status != "timeout" || writes.Load() != 1 {
		t.Fatalf("write retried or wrong timeout: %+v, %d", result, writes.Load())
	}
}
