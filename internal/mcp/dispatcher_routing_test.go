// Copyright byteyang. All Rights Reserved.

package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/bytepine/NexusDesktop/internal/unreal"
	"github.com/gorilla/websocket"
)

// 使用本地协议替身验证真实 Manager/Dispatcher 链路，不启动 UE。
func routingInstance(t *testing.T, name string) (int, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
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
			var request map[string]interface{}
			if conn.ReadJSON(&request) != nil {
				return
			}
			result := map[string]interface{}{}
			if request["method"] == "tools/call" {
				calls.Add(1)
				text, _ := json.Marshal(map[string]string{"source": name})
				result["content"] = []interface{}{map[string]interface{}{"type": "text", "text": string(text)}}
			}
			if conn.WriteJSON(map[string]interface{}{"jsonrpc": "2.0", "id": request["id"], "result": result}) != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	endpoint, _ := url.Parse(server.URL)
	port, _ = strconv.Atoi(endpoint.Port())
	return port, &calls
}

func TestDispatcherInstanceRouting(t *testing.T) {
	a, aCalls := routingInstance(t, "A")
	b, bCalls := routingInstance(t, "B")
	manager := unreal.NewManager()
	t.Cleanup(manager.Disconnect)
	if !manager.ConnectTo(a, true) {
		t.Fatal("connect A")
	}
	dispatcher := NewDispatcher(manager, nil, "test")
	dispatcher.state = stateRunning
	call := func(capability string, port int) string {
		t.Helper()
		args := map[string]interface{}{"Capability": capability, "Arguments": map[string]interface{}{"AssetPath": "/synthetic/BP"}}
		if port > 0 {
			args["targetPort"] = port
		}
		request, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]interface{}{"name": "CallCapability", "arguments": args}})
		response, err := dispatcher.Dispatch(string(request))
		if err != nil {
			t.Fatal(err)
		}
		var rpc struct {
			Result struct {
				Content []struct{ Text string }
			}
		}
		if json.Unmarshal([]byte(response), &rpc) != nil || len(rpc.Result.Content) == 0 {
			t.Fatalf("bad response %s", response)
		}
		var payload struct{ Source string }
		if json.Unmarshal([]byte(rpc.Result.Content[0].Text), &payload) != nil {
			t.Fatal("invalid content")
		}
		return payload.Source
	}
	if call("GetAssetBlueprint", 0) != "A" || call("GetAssetBlueprint", b) != "B" {
		t.Fatal("explicit route crossed instances")
	}
	if !manager.ConnectTo(b, true) || call("GetAssetBlueprint", 0) != "B" || bCalls.Load() != 1 {
		t.Fatal("default route did not reuse only B cache")
	}
	call("ManageAssetBlueprint", 0)
	if call("GetAssetBlueprint", a) != "A" || aCalls.Load() != 1 {
		t.Fatal("write B invalidated A")
	}
	if call("GetAssetBlueprint", 0) != "B" || bCalls.Load() != 3 {
		t.Fatal("write B did not invalidate B")
	}
	manager.Disconnect()
	if manager.CacheScope() != unreal.InstanceKey("", b) {
		t.Fatal("disconnect lost same-instance snapshot scope")
	}
}
