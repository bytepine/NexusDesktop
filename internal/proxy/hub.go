// Copyright byteyang. All Rights Reserved.

package proxy

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bytepine/NexusDesktop/internal/log"
)

const (
	offloadMaxAge  = time.Hour
	offloadDirPerm = 0o700
)

var offloadRoot = filepath.Join(os.TempDir(), "nexus-mcp-offload-v2")

type CacheEntry struct {
	ExactKey      string
	IdentityKey   string
	SectionKey    string
	CacheScope    string
	Capability    string
	Identity      string
	Sections      []string
	Result        interface{}
	StoredAtMS    int64
	SnapshotAtISO string
	TTLMS         int64
}

type LookupHit struct {
	Result     interface{}
	Kind       CacheKind
	SnapshotAt string
}

type GatePrompter func(CallInfo) GateDecision

// Hub 进程级会话枢纽。
type Hub struct {
	mu                  sync.Mutex
	writeGate           WriteGateMode
	paused              bool
	pauseWait           *sync.Cond
	cache               []*CacheEntry // LRU：尾部最新
	pendingEditorWrites map[string]map[string]struct{}
	cacheDisabled       bool
	// 写入失效前发出的读取即使晚到，也不能重新填回旧快照。
	cacheEpoch uint64
	prompter   GatePrompter
	offloadDir string
}

func NewHub() *Hub {
	h := &Hub{
		writeGate:           GateDestructive,
		pendingEditorWrites: make(map[string]map[string]struct{}),
		offloadDir:          filepath.Join(offloadRoot, rand.Text()),
	}
	h.pauseWait = sync.NewCond(&h.mu)
	pruneOffload(offloadMaxAge)
	return h
}

func (h *Hub) WriteGate() WriteGateMode {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.writeGate
}

func (h *Hub) SetWriteGate(mode WriteGateMode) {
	h.mu.Lock()
	h.writeGate = mode
	h.mu.Unlock()
}

func (h *Hub) SetPrompter(fn GatePrompter) {
	h.mu.Lock()
	h.prompter = fn
	h.mu.Unlock()
}

func (h *Hub) IsPaused() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.paused
}

func (h *Hub) SetPaused(v bool) {
	h.mu.Lock()
	if h.paused == v {
		h.mu.Unlock()
		return
	}
	h.paused = v
	log.Infof("Agent 转发暂停状态 paused=%t", v)
	if !v {
		h.pauseWait.Broadcast()
	}
	h.mu.Unlock()
}

func (h *Hub) WaitIfPaused() {
	h.mu.Lock()
	for h.paused {
		h.pauseWait.Wait()
	}
	h.mu.Unlock()
}

// ConfirmIfNeeded 需要确认时调用 prompter。
// DecisionAlways 原样返回，由当前 MCP 会话自己记住；Hub 不跨会话缓存。
func (h *Hub) ConfirmIfNeeded(info CallInfo) GateDecision {
	h.mu.Lock()
	mode := h.writeGate
	prompter := h.prompter
	h.mu.Unlock()
	if !NeedsGate(mode, info.Capability, info.InnerArgs) {
		return DecisionAllow
	}
	if prompter == nil {
		return DecisionAllow
	}
	log.Infof("等待写入确认 capability=%.80q timeout_ms=%d", info.Capability, GateTimeoutMS)
	ch := make(chan GateDecision, 1)
	go func() { ch <- prompter(info) }()
	select {
	case d := <-ch:
		log.Infof("写入确认结束 capability=%.80q decision=%v", info.Capability, d)
		return d
	case <-time.After(time.Duration(GateTimeoutMS) * time.Millisecond):
		log.Warnf("写入确认超时 capability=%.80q timeout_ms=%d", info.Capability, GateTimeoutMS)
		return DecisionDeny
	}
}

func (h *Hub) LookupFresh(info CallInfo, nowMS int64) *LookupHit {
	if info.IsWrite || info.IsBatch || IsEditorOperation(info.Capability) {
		return nil
	}
	return h.lookup(info, nowMS, false)
}

func (h *Hub) LookupDegraded(info CallInfo, nowMS int64) *LookupHit {
	if info.IsWrite || info.IsBatch || IsEditorOperation(info.Capability) {
		return nil
	}
	return h.lookup(info, nowMS, true)
}

func (h *Hub) CacheGeneration() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cacheEpoch
}

func (h *Hub) Store(info CallInfo, result interface{}, nowMS int64, generation ...uint64) interface{} {
	stored := cloneJSON(result)
	if IsEditorOperation(info.Capability) {
		h.observeEditorOperation(info, stored)
		return stored
	}
	// 项目级写入无法按资产定位；部分失败也清理本实例，避免旧快照误导读回。
	if info.IsBatch {
		h.invalidateIdentity("", info.CacheScope)
	} else if info.IsWrite {
		h.invalidateIdentity(info.Identity, info.CacheScope)
	}
	if rec, ok := stored.(map[string]interface{}); ok && rec["isError"] == true {
		return stored
	}
	ttl, snap := ExtractTtlMS(stored, info.Capability, nowMS)
	hasImage := false
	if rec, ok := stored.(map[string]interface{}); ok {
		content, _ := rec["content"].([]interface{})
		for _, item := range content {
			if block, ok := item.(map[string]interface{}); ok && block["type"] == "image" {
				hasImage = true
				break
			}
		}
	}
	if ContentTextLen(stored) > OffloadChars && !hasImage {
		stored = h.offload(stored)
	}
	if ttl == 0 {
		// 同一读取对象的旧 All/section/degraded 快照也不能继续复用。
		h.mu.Lock()
		out := h.cache[:0]
		for _, entry := range h.cache {
			if entry.IdentityKey != info.IdentityKey {
				out = append(out, entry)
			}
		}
		h.cache = out
		h.mu.Unlock()
		return stored
	}
	if !info.IsWrite && !info.IsBatch && info.Capability != "" && !hasImage {
		if snap == "" {
			snap = time.UnixMilli(nowMS).UTC().Format(time.RFC3339)
		}
		h.mu.Lock()
		if _, blocked := h.pendingEditorWrites[info.CacheScope]; h.cacheDisabled || blocked || (len(generation) > 0 && generation[0] != h.cacheEpoch) {
			h.mu.Unlock()
			return stored
		}
		h.cache = append(h.cache, &CacheEntry{
			ExactKey: info.ExactKey, IdentityKey: info.IdentityKey, SectionKey: info.SectionKey, CacheScope: info.CacheScope,
			Capability: info.Capability, Identity: info.Identity, Sections: info.Sections,
			Result: cloneJSON(stored), StoredAtMS: nowMS, SnapshotAtISO: snap, TTLMS: ttl,
		})
		for len(h.cache) > MaxCacheEntries {
			h.cache = h.cache[1:]
		}
		h.mu.Unlock()
	}
	return stored
}

func (h *Hub) lookup(info CallInfo, nowMS int64, allowExpired bool) *LookupHit {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, blocked := h.pendingEditorWrites[info.CacheScope]; h.cacheDisabled || blocked {
		return nil
	}
	for i := len(h.cache) - 1; i >= 0; i-- {
		e := h.cache[i]
		if e.ExactKey == info.ExactKey && h.usable(e, nowMS, allowExpired, info.Capability) {
			return &LookupHit{Result: cloneJSON(e.Result), Kind: CacheHit, SnapshotAt: e.SnapshotAtISO}
		}
	}
	if info.Identity != "" && len(info.Sections) > 0 {
		for i := len(h.cache) - 1; i >= 0; i-- {
			e := h.cache[i]
			if e.SectionKey != info.SectionKey {
				continue
			}
			if !SectionsCovered(e.Sections, info.Sections) {
				continue
			}
			within := nowMS-e.StoredAtMS <= SectionWindowMS
			if !within && !allowExpired {
				continue
			}
			if !h.usable(e, nowMS, allowExpired || within, info.Capability) {
				continue
			}
			return &LookupHit{Result: cloneJSON(e.Result), Kind: CacheSectionHit, SnapshotAt: e.SnapshotAtISO}
		}
	}
	return nil
}

func (h *Hub) observeEditorOperation(info CallInfo, result interface{}) {
	envelope, _ := result.(map[string]interface{})
	source, _ := envelope["structuredContent"].(map[string]interface{})
	if source == nil {
		source = resultTextObject(result)
	}
	if source == nil {
		source = envelope
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if info.IsWrite {
		h.invalidateIdentityLocked("", info.CacheScope)
	}
	observe := func(snapshot, defaults map[string]interface{}) {
		id, _ := snapshot["OperationId"].(string)
		if id == "" {
			id, _ = defaults["OperationId"].(string)
		}
		state, ok := snapshot["State"].(string)
		if !ok {
			state, _ = defaults["State"].(string)
		}
		if id == "" {
			return
		}
		if info.IsWrite && state == "Running" && !h.cacheDisabled {
			if _, found := h.pendingEditorWrites[info.CacheScope]; !found {
				// ponytail: 跟踪有界；超限后保守停缓存，重启代理恢复，不能驱逐尚未确认结束的写入。
				if len(h.pendingEditorWrites) >= MaxCacheEntries {
					h.cacheDisabled = true
					clear(h.pendingEditorWrites)
					h.cache = nil
					return
				}
				h.pendingEditorWrites[info.CacheScope] = make(map[string]struct{})
			}
			pending := h.pendingEditorWrites[info.CacheScope]
			if pending == nil {
				return
			}
			if _, exists := pending[id]; len(pending) >= 128 && !exists {
				h.pendingEditorWrites[info.CacheScope] = nil
			} else {
				pending[id] = struct{}{}
			}
		} else if state == "Completed" || state == "Failed" || state == "Cancelled" || state == "Interrupted" {
			pending := h.pendingEditorWrites[info.CacheScope]
			if _, found := pending[id]; found {
				delete(pending, id)
				h.invalidateIdentityLocked("", info.CacheScope)
				if len(pending) == 0 {
					delete(h.pendingEditorWrites, info.CacheScope)
				}
			}
		}
	}
	observe(source, nil)
	defaults, _ := source["Operations_defaults"].(map[string]interface{})
	operations, _ := source["Operations"].([]interface{})
	for _, item := range operations {
		if snapshot, ok := item.(map[string]interface{}); ok {
			observe(snapshot, defaults)
		}
	}
}

func (h *Hub) usable(e *CacheEntry, nowMS int64, allowExpired bool, cap string) bool {
	if nowMS-e.StoredAtMS <= e.TTLMS {
		return true
	}
	if !allowExpired {
		return false
	}
	return IsDurableRead(cap)
}

func (h *Hub) invalidateIdentity(identity, cacheScope string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.invalidateIdentityLocked(identity, cacheScope)
}

func (h *Hub) invalidateIdentityLocked(identity, cacheScope string) {
	h.cacheEpoch++
	out := h.cache[:0]
	for _, e := range h.cache {
		if e.CacheScope != cacheScope || (identity != "" && e.Identity != identity) {
			out = append(out, e)
		}
	}
	h.cache = out
}

func (h *Hub) offload(result interface{}) interface{} {
	pruneOffload(offloadMaxAge)
	if err := os.MkdirAll(h.offloadDir, offloadDirPerm); err != nil {
		log.Warnf("大响应落盘失败 stage=mkdir，保留原响应: %v", err)
		return result
	}
	b, err := json.Marshal(result)
	if err != nil {
		log.Warnf("大响应落盘失败 stage=encode，保留原响应: %v", err)
		return result
	}
	f, err := os.CreateTemp(h.offloadDir, "offload-*.json")
	if err != nil {
		log.Warnf("大响应落盘失败 stage=create，保留原响应: %v", err)
		return result
	}
	file := f.Name()
	_, writeErr := f.Write(b)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		log.Warnf("大响应落盘失败 stage=write，保留原响应 write_err=%v close_err=%v", writeErr, closeErr)
		_ = os.Remove(file)
		return result
	}
	log.Infof("大响应已落盘 bytes=%d file=%q", len(b), file)
	summary := map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": mustJSON(map[string]interface{}{"summary": "payload offloaded", "path": file, "bytes": len(b)}),
		}},
		"isError": false,
	}
	return InjectProxyMeta(summary, map[string]interface{}{"offloaded": true, "path": file, "bytes": len(b)})
}

func pruneOffload(maxAge time.Duration) {
	pruneOffloadDir(offloadRoot, time.Now().Add(-maxAge))
}

func pruneOffloadDir(dir string, cutoff time.Time) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			pruneOffloadDir(path, cutoff)
			_ = os.Remove(path) // 仅删除空目录，保留其他实例的新文件。
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
		}
	}
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
