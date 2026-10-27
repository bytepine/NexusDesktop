// Copyright byteyang. All Rights Reserved.
package unreal

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestOperationProgressCancelAndResponseMatching(t *testing.T) {
	upgrade := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrade.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		var request map[string]interface{}
		if conn.ReadJSON(&request) != nil {
			return
		}
		id := request["id"]
		_ = conn.WriteJSON(map[string]interface{}{"jsonrpc": "2.0", "id": -100, "result": map[string]interface{}{"wrong": true}})
		_ = conn.WriteJSON(map[string]interface{}{"jsonrpc": "2.0", "method": "notifications/progress", "params": map[string]interface{}{"progressToken": "token", "progress": 1}})
		var cancel map[string]interface{}
		if conn.ReadJSON(&cancel) != nil {
			return
		}
		params, _ := cancel["params"].(map[string]interface{})
		if cancel["method"] != "notifications/cancelled" || params["requestId"] != id {
			return
		}
		_ = conn.WriteJSON(map[string]interface{}{"jsonrpc": "2.0", "id": id, "result": map[string]interface{}{"cancelled": true}})
	}))
	defer server.Close()
	port, _ := strconv.Atoi(strings.TrimPrefix(server.URL, "http://127.0.0.1:"))
	manager := NewManager()
	defer manager.Disconnect()
	manager.Instances = []InstanceInfo{{Port: port, WsPort: port, Host: LoopbackHost}}
	cancelled := make(chan struct{})
	count := 0
	result := manager.ForwardOperation(port, map[string]interface{}{"name": "GetEditorOperation", "_meta": map[string]interface{}{"nexusSessionId": "session"}}, func(event map[string]interface{}) { count++; close(cancelled) }, cancelled)
	if result.Status != "ok" || count != 1 {
		t.Fatalf("missing progress or final result: %+v, %d", result, count)
	}
	raw, _ := json.Marshal(result.Response)
	if !strings.Contains(string(raw), `"cancelled":true`) {
		t.Fatalf("wrong response matched: %s", raw)
	}
}
