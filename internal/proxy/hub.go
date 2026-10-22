// Copyright byteyang. All Rights Reserved.

package proxy

import (
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	offloadMaxAge  = time.Hour
	offloadDirPerm = 0o700
)

var offloadRoot = filepath.Join(os.TempDir(), "nexus-mcp-offload-v2")

type CacheEntry struct {
	ExactKey      string
	IdentityKey   string
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
	mu         sync.Mutex
	writeGate  WriteGateMode
	paused     bool
	pauseWait  *sync.Cond
	cache      []*CacheEntry // LRU：尾部最新
	prompter   GatePrompter
	offloadDir string
}

func NewHub() *Hub {
	h := &Hub{
		writeGate:  GateDestructive,
		offloadDir: filepath.Join(offloadRoot, rand.Text()),
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
	ch := make(chan GateDecision, 1)
	go func() { ch <- prompter(info) }()
	select {
	case d := <-ch:
		return d
	case <-time.After(time.Duration(GateTimeoutMS) * time.Millisecond):
		return DecisionDeny
	}
}

func (h *Hub) LookupFresh(info CallInfo, nowMS int64) *LookupHit {
	if info.IsWrite || info.IsBatch {
		return nil
	}
	return h.lookup(info, nowMS, false)
}

func (h *Hub) LookupDegraded(info CallInfo, nowMS int64) *LookupHit {
	if info.IsWrite || info.IsBatch {
		return nil
	}
	return h.lookup(info, nowMS, true)
}

func (h *Hub) Store(info CallInfo, result interface{}, nowMS int64) interface{} {
	stored := cloneJSON(result)
	// 批量操作可能部分成功，先失效目标实例，避免保留旧快照。
	if info.IsBatch {
		h.invalidateIdentity("", info.CacheScope)
	} else if info.IsWrite && info.Identity != "" {
		h.invalidateIdentity(info.Identity, info.CacheScope)
	}
	if rec, ok := stored.(map[string]interface{}); ok && rec["isError"] == true {
		return stored
	}
	ttl, snap := ExtractTtlMS(stored, info.Capability, nowMS)
	if ContentTextLen(stored) > OffloadChars {
		stored = h.offload(stored)
	}
	if !info.IsWrite && !info.IsBatch && info.Capability != "" {
		if snap == "" {
			snap = time.UnixMilli(nowMS).UTC().Format(time.RFC3339)
		}
		h.mu.Lock()
		h.cache = append(h.cache, &CacheEntry{
			ExactKey: info.ExactKey, IdentityKey: info.IdentityKey, CacheScope: info.CacheScope,
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
	for i := len(h.cache) - 1; i >= 0; i-- {
		e := h.cache[i]
		if e.ExactKey == info.ExactKey && h.usable(e, nowMS, allowExpired, info.Capability) {
			return &LookupHit{Result: cloneJSON(e.Result), Kind: CacheHit, SnapshotAt: e.SnapshotAtISO}
		}
	}
	if info.Identity != "" && len(info.Sections) > 0 {
		for i := len(h.cache) - 1; i >= 0; i-- {
			e := h.cache[i]
			if e.IdentityKey != info.IdentityKey {
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
		return result
	}
	b, err := json.Marshal(result)
	if err != nil {
		return result
	}
	f, err := os.CreateTemp(h.offloadDir, "offload-*.json")
	if err != nil {
		return result
	}
	file := f.Name()
	_, writeErr := f.Write(b)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(file)
		return result
	}
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
