// Copyright byteyang. All Rights Reserved.
package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
)

type operationRequest struct {
	cancel   chan struct{}
	once     sync.Once
	progress func(map[string]interface{})
}

func newOperationSession() string {
	var value [16]byte
	_, _ = rand.Read(value[:])
	return hex.EncodeToString(value[:])
}
func operationRequestKey(id interface{}) string { value, _ := json.Marshal(id); return string(value) }
func cloneMapMap(value interface{}) map[string]interface{} {
	source, _ := value.(map[string]interface{})
	result := make(map[string]interface{})
	for key, item := range source {
		result[key] = item
	}
	return result
}
func isEditorOperation(name string) bool {
	switch name {
	case "GetEditorSlate", "CaptureEditorSlate", "InteractEditorSlate", "ControlEditorAutomation", "ListEditorAutomationTests", "CompileEditorLiveCoding", "GetEditorOperation":
		return true
	}
	return false
}

func (d *Dispatcher) Dispatch(body string) (string, error) { return d.DispatchWithProgress(body, nil) }

// 每个请求独立持有回调；相同客户端 id 不会跨 MCP 会话复用。
func (d *Dispatcher) DispatchWithProgress(body string, notify func(map[string]interface{})) (string, error) {
	var message map[string]interface{}
	if json.Unmarshal([]byte(body), &message) != nil {
		return d.dispatch(body)
	}
	params, _ := message["params"].(map[string]interface{})
	if message["method"] == "notifications/cancelled" {
		if request, found := d.requests.Load(operationRequestKey(params["requestId"])); found {
			item := request.(*operationRequest)
			item.once.Do(func() { close(item.cancel) })
		}
		return "", nil
	}
	if message["method"] != "tools/call" {
		return d.dispatch(body)
	}
	key := operationRequestKey(message["id"])
	request := &operationRequest{cancel: make(chan struct{}), progress: notify}
	if _, loaded := d.requests.LoadOrStore(key, request); loaded {
		return makeError(message["id"], errInvalidRequest, "Duplicate active request id"), nil
	}
	defer d.requests.Delete(key)
	return d.dispatch(body)
}
