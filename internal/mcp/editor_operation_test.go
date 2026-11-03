// Copyright byteyang. All Rights Reserved.
package mcp

import (
	"encoding/json"
	"testing"

	"github.com/bytepine/NexusDesktop/internal/unreal"
)

func mustOperationJSON(id string) string {
	value, _ := json.Marshal(map[string]string{"OperationId": id})
	return string(value)
}

func TestEditorOperationOwnerAndCacheRouting(t *testing.T) {
	port, calls := routingInstance(t, "operations")
	manager := unreal.NewManager()
	t.Cleanup(manager.Dispose)
	if !manager.ConnectTo(port, true) {
		t.Fatal("connect mock UE")
	}
	first, second := NewDispatcher(manager, nil, "test"), NewDispatcher(manager, nil, "test")
	first.state, second.state = stateRunning, stateRunning
	type content struct{ Text string }
	type result struct {
		IsError bool
		Content []content
	}
	id := 0
	call := func(dispatcher *Dispatcher, capability string, args map[string]interface{}, direct, explicit bool) result {
		t.Helper()
		name := capability
		arguments := cloneMap(args)
		if !direct {
			name = "CallCapability"
			arguments = map[string]interface{}{"Capability": capability, "Arguments": args}
		}
		if explicit {
			arguments["targetPort"] = port
		}
		id++
		body, _ := json.Marshal(map[string]interface{}{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]interface{}{"name": name, "arguments": arguments}})
		response, err := dispatcher.Dispatch(string(body))
		var rpc struct {
			Result result
			Error  interface{}
		}
		if err != nil || json.Unmarshal([]byte(response), &rpc) != nil || rpc.Error != nil || len(rpc.Result.Content) == 0 {
			t.Fatalf("bad response: %s (%v)", response, err)
		}
		return rpc.Result
	}
	for _, capability := range []string{"GetEditorSlate", "CaptureEditorSlate", "InteractEditorSlate", "ControlEditorAutomation", "ListEditorAutomationTests", "CompileEditorLiveCoding",
		"GetEditorPCG", "ControlEditorPCG", "GetEditorNiagara", "ExecNiagaraStackFix", "SearchAssetsSemantic", "GetProjectSkills",
		"GetEditorAnimation", "ControlEditorAnimation", "ControlEditorNiagara", "ControlEditorGameFeature", "GetEditorScene", "ControlEditorScene"} {
		t.Run(capability, func(t *testing.T) {
			for _, direct := range []bool{false, true} {
				for _, explicit := range []bool{false, true} {
					started := call(first, capability, map[string]interface{}{}, direct, explicit)
					var operation struct{ OperationId string }
					if json.Unmarshal([]byte(started.Content[0].Text), &operation) != nil || operation.OperationId == "" {
						t.Fatal("missing operation id")
					}
					args := map[string]interface{}{"OperationId": operation.OperationId}
					if call(first, "GetEditorOperation", args, direct, explicit).IsError {
						t.Errorf("owner changed (direct=%v explicit=%v)", direct, explicit)
					}
					if !call(second, "GetEditorOperation", args, direct, explicit).IsError {
						t.Error("operation crossed MCP sessions")
					}
					before := calls.Load()
					call(first, capability, map[string]interface{}{}, direct, explicit)
					if calls.Load() != before+1 {
						t.Error("task response was cached")
					}
				}
			}
		})
	}
}
