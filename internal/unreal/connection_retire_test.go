// Copyright byteyang. All Rights Reserved.
package unreal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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

func TestScanMissPreservesConnectedEditorAndPendingCall(t *testing.T) {
	upgrader := websocket.Upgrader{}
	stop := make(chan struct{})
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
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// 扫描端口仅返回普通 HTTP 错误，模拟忙碌编辑器不出现在探测结果中。
	probe := httptest.NewServer(http.NotFoundHandler())
	defer probe.Close()
	address, _ := url.Parse(probe.URL)
	port, _ := strconv.Atoi(address.Port())
	m := NewManager()
	m.ws, m.ConnectedPort = conn, port
	m.ScanPortStart, m.ScanPortEnd = port, port
	pending := make(chan WsRequestResult, 1)
	m.pendingRequests[42] = pending
	m.DiscoverInstances()
	if snapshot := m.Snapshot(); !snapshot.WsOpen || snapshot.ConnectedPort != port {
		t.Fatalf("scan miss retired the editor: %+v", snapshot)
	}
	select {
	case result := <-pending:
		t.Fatalf("scan canceled an in-flight write: %+v", result)
	default:
	}
}

func TestMissingPreferredEditorDoesNotSelectRuntime(t *testing.T) {
	probes := 0
	runtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		probes++
		fmt.Fprint(w, `{"server":"NexusLink","hostKind":"Game","projectName":"Test"}`)
	}))
	defer runtime.Close()
	address, _ := url.Parse(runtime.URL)
	port, _ := strconv.Atoi(address.Port())
	m := NewManager()
	m.ScanPortStart, m.ScanPortEnd = port, port
	m.PreferredPort, m.PreferredHost = port+1, LoopbackHost
	m.DiscoverInstances()
	if snapshot := m.Snapshot(); snapshot.WsOpen || snapshot.ConnectedPort > 0 || probes != 1 {
		t.Fatalf("missing preferred editor attempted runtime connection: %+v probes=%d", snapshot, probes)
	}
}
