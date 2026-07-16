// Copyright byteyang. All Rights Reserved.

package unreal

import (
	"strings"
	"sync"
)

// ProxyFeedbackCategory 代理层失败分类（对应 UE `nexus/proxy_feedback` 的 category 参数）。
type ProxyFeedbackCategory string

const (
	ProxyFeedbackTimeout     ProxyFeedbackCategory = "proxy_timeout"
	ProxyFeedbackDisconnect  ProxyFeedbackCategory = "proxy_disconnect"
	ProxyFeedbackConnectFail ProxyFeedbackCategory = "proxy_connect_fail"
)

// ProxyFeedbackEvent 一条待上报的代理层失败事件。
type ProxyFeedbackEvent struct {
	Category  ProxyFeedbackCategory
	Tool      string
	ErrorText string
	Note      string
}

const proxyFeedbackMaxSize = 50

// proxyFeedbackBuffer 代理层失败事件的进程内环形缓冲。
// 断连时先入队，待连上 UE 后由 flush 逐条经 nexus/proxy_feedback 上报。
// 旧版 NexusLink（未实现该 method）返回 Method not found 后标记 unsupported，
// 之后静默跳过，不再发送、不再入队——保证新代理连旧 UE 时零感知报错。
type proxyFeedbackBuffer struct {
	mu          sync.Mutex
	queue       []ProxyFeedbackEvent
	unsupported bool
}

func newProxyFeedbackBuffer() *proxyFeedbackBuffer {
	return &proxyFeedbackBuffer{}
}

func (b *proxyFeedbackBuffer) enqueue(event ProxyFeedbackEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.unsupported {
		return
	}
	b.queue = append(b.queue, event)
	if len(b.queue) > proxyFeedbackMaxSize {
		b.queue = b.queue[len(b.queue)-proxyFeedbackMaxSize:]
	}
}

// drain 取出并清空当前所有待发事件。
func (b *proxyFeedbackBuffer) drain() []ProxyFeedbackEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	events := b.queue
	b.queue = nil
	return events
}

// requeue 把未 flush 成功的事件放回队首，供下次连上后重试。
func (b *proxyFeedbackBuffer) requeue(event ProxyFeedbackEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.unsupported {
		return
	}
	b.queue = append([]ProxyFeedbackEvent{event}, b.queue...)
	if len(b.queue) > proxyFeedbackMaxSize {
		b.queue = b.queue[:proxyFeedbackMaxSize]
	}
}

func (b *proxyFeedbackBuffer) hasPending() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.queue) > 0
}

func (b *proxyFeedbackBuffer) isUnsupported() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.unsupported
}

// markUnsupported 标记本会话 UE 不支持 proxy_feedback：清空缓冲，停止后续上报。
func (b *proxyFeedbackBuffer) markUnsupported() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.unsupported = true
	b.queue = nil
}

// isMethodNotFoundError 判定 WS 响应是否为「方法未找到」（UE 未实现 nexus/proxy_feedback，即旧版 NexusLink）。
func isMethodNotFoundError(resp map[string]interface{}) bool {
	if resp == nil {
		return false
	}
	errObj, ok := resp["error"].(map[string]interface{})
	if !ok {
		return false
	}
	if code, ok := errObj["code"].(float64); ok && code == -32601 {
		return true
	}
	msg, _ := errObj["message"].(string)
	return strings.Contains(msg, "Method not found") || strings.Contains(msg, "方法未找到")
}
