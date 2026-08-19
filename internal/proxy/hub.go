// Copyright byteyang. All Rights Reserved.

package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type CacheEntry struct {
	ExactKey      string
	IdentityKey   string
	Capability    string
	Identity      string
	Sections      []string
	Result        interface{}
	StoredAtMS    int64
	SnapshotAtISO string
	TTLMS         int64
}

type ActivityState struct {
	Capability string
	Identity   string
	AtMS       int64
	Paused     bool
}

type LookupHit struct {
	Result     interface{}
	Kind       CacheKind
	SnapshotAt string
}

type GatePrompter func(CallInfo) GateDecision

// Hub 进程级会话枢纽。
type Hub struct {
	mu          sync.Mutex
	writeGate   WriteGateMode
	paused      bool
	pauseWait   *sync.Cond
	alwaysAllow map[string]struct{}
	cache       []*CacheEntry // LRU：尾部最新
	activity    *ActivityState
	prompter    GatePrompter
	OnActivity  func()
}

func NewHub() *Hub {
	h := &Hub{
		writeGate:   GateDestructive,
		alwaysAllow: map[string]struct{}{},
	}
	h.pauseWait = sync.NewCond(&h.mu)
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
	h.emit()
}

func (h *Hub) Activity() *ActivityState {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.paused {
		return &ActivityState{Paused: true, AtMS: time.Now().UnixMilli()}
	}
	if h.activity == nil {
		return nil
	}
	cp := *h.activity
	return &cp
}

func (h *Hub) BeginCall(info CallInfo) {
	h.mu.Lock()
	h.activity = &ActivityState{Capability: info.Capability, Identity: info.Identity, AtMS: time.Now().UnixMilli()}
	h.mu.Unlock()
	h.emit()
}

func (h *Hub) EndCall() {
	h.emit()
}

func (h *Hub) WaitIfPaused() {
	h.mu.Lock()
	for h.paused {
		h.pauseWait.Wait()
	}
	h.mu.Unlock()
}

func (h *Hub) ConfirmIfNeeded(info CallInfo) GateDecision {
	h.mu.Lock()
	mode := h.writeGate
	_, always := h.alwaysAllow[info.Capability]
	prompter := h.prompter
	h.mu.Unlock()
	if !NeedsGate(mode, info.Capability, info.InnerArgs) {
		return DecisionAllow
	}
	if always {
		return DecisionAllow
	}
	if prompter == nil {
		return DecisionAllow
	}
	ch := make(chan GateDecision, 1)
	go func() { ch <- prompter(info) }()
	select {
	case d := <-ch:
		if d == DecisionAlways {
			h.mu.Lock()
			h.alwaysAllow[info.Capability] = struct{}{}
			h.mu.Unlock()
			return DecisionAllow
		}
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
	if ContentTextLen(stored) > OffloadChars {
		stored = h.offload(stored)
	}
	if !info.IsWrite && !info.IsBatch && info.Capability != "" {
		ttl, snap := ExtractTtlMS(stored, info.Capability, nowMS)
		if snap == "" {
			snap = time.UnixMilli(nowMS).UTC().Format(time.RFC3339)
		}
		h.mu.Lock()
		h.cache = append(h.cache, &CacheEntry{
			ExactKey: info.ExactKey, IdentityKey: info.IdentityKey,
			Capability: info.Capability, Identity: info.Identity, Sections: info.Sections,
			Result: cloneJSON(stored), StoredAtMS: nowMS, SnapshotAtISO: snap, TTLMS: ttl,
		})
		for len(h.cache) > MaxCacheEntries {
			h.cache = h.cache[1:]
		}
		h.mu.Unlock()
	}
	if info.IsWrite && info.Identity != "" {
		h.invalidateIdentity(info.Identity)
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

func (h *Hub) invalidateIdentity(identity string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := h.cache[:0]
	for _, e := range h.cache {
		if e.Identity != identity {
			out = append(out, e)
		}
	}
	h.cache = out
}

func (h *Hub) offload(result interface{}) interface{} {
	dir := filepath.Join(os.TempDir(), "nexus-mcp-offload")
	_ = os.MkdirAll(dir, 0o755)
	file := filepath.Join(dir, "offload-"+strconv.FormatInt(time.Now().UnixMilli(), 10)+".json")
	b, _ := json.Marshal(result)
	_ = os.WriteFile(file, b, 0o644)
	summary := map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": mustJSON(map[string]interface{}{"summary": "payload offloaded", "path": file, "bytes": len(b)}),
		}},
		"isError": false,
	}
	return InjectProxyMeta(summary, map[string]interface{}{"offloaded": true, "path": file, "bytes": len(b)})
}

func mustJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (h *Hub) emit() {
	if h.OnActivity != nil {
		h.OnActivity()
	}
}
