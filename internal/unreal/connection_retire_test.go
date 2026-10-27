// Copyright byteyang. All Rights Reserved.
package unreal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func TestRetiredSocketPreservesCurrentConnection(t *testing.T) {
	stop := make(chan struct{})
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-stop
	}))
	defer server.Close()
	defer close(stop)
	dial := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
		if err != nil {
			t.Fatal(err)
		}
		return conn
	}
	old := dial()
	current := dial()
	defer current.Close()
	_ = old.Close()
	m := NewManager()
	m.ws, m.connectionEpoch, m.ConnectedPort = current, 1, 45000
	m.ScanPortStart, m.ScanPortEnd = 1, 0
	m.manuallyDisconnected = true // 失败时也不向测试以外的服务发起连接。
	pending := make(chan WsRequestResult, 1)
	m.pendingRequests[42] = pending
	keepaliveCanceled := false
	m.keepaliveCancel = func() { keepaliveCanceled = true }
	// 即使 epoch 尚未更新，已被替换的 socket 也不能重置当前状态。
	m.receiveLoop(old, 1)
	if snapshot := m.Snapshot(); !snapshot.WsOpen || snapshot.ConnectedPort != 45000 || keepaliveCanceled {
		t.Fatalf("retired socket changed current connection: %+v, keepaliveCanceled=%v", snapshot, keepaliveCanceled)
	}
	select {
	case result := <-pending:
		t.Fatalf("retired socket canceled a current request: %+v", result)
	default:
	}
}
