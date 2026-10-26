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

	"github.com/bytepine/NexusDesktop/internal/proxy"
	"github.com/bytepine/NexusDesktop/internal/unreal"
	"github.com/gorilla/websocket"
)

// 使用本地协议替身验证真实 Manager/Dispatcher 链路，不启动 UE。
func routingInstance(t *testing.T, name string, toolNames ...string) (int, *atomic.Int32) {
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
			if request["method"] == "auth" {
				params, _ := request["params"].(map[string]interface{})
				result["ok"] = params["token"] == "contract-test-token"
			}
			if request["method"] == "tools/list" && len(toolNames) > 0 {
				tools := make([]interface{}, 0, len(toolNames))
				for _, toolName := range toolNames {
					tools = append(tools, map[string]interface{}{"name": toolName, "inputSchema": map[string]interface{}{"type": "object"}})
				}
				result["tools"] = tools
			}
			if request["method"] == "tools/call" {
				if len(toolNames) > 0 {
					params, _ := request["params"].(map[string]interface{})
					found := false
					for _, toolName := range toolNames {
						found = found || params["name"] == toolName
					}
					if !found {
						_ = conn.WriteJSON(map[string]interface{}{"jsonrpc": "2.0", "id": request["id"], "error": map[string]interface{}{"code": -32601, "message": "Tool not found"}})
						continue
					}
				}
				calls.Add(1)
				text, _ := json.Marshal(map[string]string{"source": name})
				result["content"] = []interface{}{map[string]interface{}{"type": "text", "text": string(text)}}
				if name == "failure" {
					result["isError"] = true
					result["structuredContent"] = map[string]interface{}{"ErrorKind": "ArgInvalid", "bRolledBack": false, "Error": "contract failure"}
				}
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

func TestStructuredFailureIsForwardedOnce(t *testing.T) {
	port, calls := routingInstance(t, "failure", "CallCapability")
	manager := unreal.NewManager()
	t.Cleanup(manager.Disconnect)
	if !manager.ConnectTo(port, true) {
		t.Fatal("connect mock UE")
	}
	dispatcher := NewDispatcher(manager, nil, "test")
	dispatcher.state = stateRunning
	response, err := dispatcher.Dispatch(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"CallCapability","arguments":{"Capability":"ManageAssetDataTable","Arguments":{"AssetPath":"/synthetic/DT"}}}}`)
	var rpc struct {
		Result struct {
			IsError           bool
			StructuredContent map[string]interface{}
		}
	}
	if err != nil || json.Unmarshal([]byte(response), &rpc) != nil || !rpc.Result.IsError || rpc.Result.StructuredContent["ErrorKind"] != "ArgInvalid" || calls.Load() != 1 {
		t.Fatalf("structured failure must survive exactly one write: %s (%v), calls=%d", response, err, calls.Load())
	}
}

func TestDispatcherRefreshesToolNamesAfterReconnect(t *testing.T) {
	legacyNames := []string{"search_capabilities", "call_capability", "submit_feedback"}
	modernNames := []string{"SearchCapabilities", "CallCapability", "SubmitFeedback"}
	legacy, _ := routingInstance(t, "legacy", legacyNames...)
	modern, calls := routingInstance(t, "modern", modernNames...)
	manager := unreal.NewManager()
	t.Cleanup(manager.Disconnect)
	manager.ScanPortStart, manager.ScanPortEnd = modern, modern
	dispatcher := NewDispatcher(manager, nil, "test")
	dispatcher.state = stateRunning
	list := func() map[string]bool {
		t.Helper()
		response, err := dispatcher.Dispatch(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		var rpc struct {
			Result struct{ Tools []struct{ Name string } }
		}
		if err != nil || json.Unmarshal([]byte(response), &rpc) != nil {
			t.Fatalf("tools/list: %s (%v)", response, err)
		}
		names := map[string]bool{}
		for _, tool := range rpc.Result.Tools {
			names[tool.Name] = true
		}
		return names
	}
	if !manager.ConnectTo(legacy, true) {
		t.Fatal("connect legacy")
	}
	for _, name := range legacyNames {
		if !list()[name] {
			t.Fatalf("legacy tool missing: %s", name)
		}
	}
	if !manager.ConnectTo(modern, true) {
		t.Fatal("reconnect modern")
	}
	updated := list()
	for index, name := range modernNames {
		if !updated[name] || updated[legacyNames[index]] {
			t.Fatalf("stale tool list after reconnect: %v", updated)
		}
		params := map[string]interface{}{"name": name, "arguments": map[string]interface{}{"Query": "", "Capability": "GetEditorInfo", "Arguments": map[string]interface{}{}}}
		request, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": params})
		response, err := dispatcher.Dispatch(string(request))
		var rpc map[string]interface{}
		if err != nil || json.Unmarshal([]byte(response), &rpc) != nil || rpc["error"] != nil {
			t.Fatalf("new tool failed: %s (%v)", response, err)
		}
	}
	if calls.Load() != 3 {
		t.Fatalf("new tools must actually reach the new UE: calls=%d", calls.Load())
	}
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
	prompts := 0
	manager.Hub.SetPrompter(func(proxy.CallInfo) proxy.GateDecision {
		prompts++
		return proxy.DecisionAllow
	})
	for _, port := range []interface{}{-1, 0, 1023, 65536, 45000.5, "45000", nil, true} {
		for _, capability := range []string{"GetAssetBlueprint", "ManageAssetBlueprint", "DeleteAsset"} {
			request, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": map[string]interface{}{
				"name": "CallCapability", "arguments": map[string]interface{}{"Capability": capability, "Arguments": map[string]interface{}{"AssetPath": "/synthetic/BP"}, "targetPort": port},
			}})
			response, err := dispatcher.Dispatch(string(request))
			var rpc struct{ Error struct{ Code int } }
			if err != nil || json.Unmarshal([]byte(response), &rpc) != nil || rpc.Error.Code != errInvalidParams {
				t.Fatalf("invalid explicit target must be rejected: %s (%v)", response, err)
			}
		}
	}
	if aCalls.Load() != 1 || bCalls.Load() != 1 || prompts != 0 {
		t.Fatal("invalid targets must not forward, serve cached reads, or prompt")
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
