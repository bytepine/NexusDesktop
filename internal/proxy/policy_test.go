// Copyright byteyang. All Rights Reserved.

package proxy

import "testing"

func TestNeedsGate(t *testing.T) {
	if !NeedsGate(GateDestructive, "delete_asset", nil) {
		t.Fatal("delete")
	}
	if NeedsGate(GateOff, "delete_asset", nil) {
		t.Fatal("off")
	}
	if !IsDestructive("control_pie", map[string]interface{}{"action": "stop"}) {
		t.Fatal("pie stop")
	}
	if IsDestructive("control_pie", map[string]interface{}{"action": "start"}) {
		t.Fatal("pie start")
	}
	if !NeedsGate(GateAll, "create_asset_blueprint", nil) {
		t.Fatal("all writes")
	}
}

func TestSectionsAndCall(t *testing.T) {
	if !SectionsCovered([]string{"all"}, []string{"graphOverview"}) {
		t.Fatal("all covers")
	}
	info := ParseCall("call_capability", map[string]interface{}{
		"capability": "get_asset_blueprint",
		"arguments":  map[string]interface{}{"assetPath": "/Game/BP", "sections": []interface{}{"graphOverview"}},
	})
	if info.Capability != "get_asset_blueprint" || info.Identity != "/Game/BP" || info.IsWrite {
		t.Fatalf("%+v", info)
	}
}

func TestHubCache(t *testing.T) {
	h := NewHub()
	now := int64(1_000_000)
	info := ParseCall("call_capability", map[string]interface{}{
		"capability": "get_asset_blueprint",
		"arguments":  map[string]interface{}{"assetPath": "/Game/BP", "sections": []interface{}{"all"}},
	})
	payload := map[string]interface{}{
		"content": []interface{}{map[string]interface{}{
			"type": "text",
			"text": `{"graph":1,"_ttl_seconds":30}`,
		}},
		"isError": false,
	}
	h.Store(info, payload, now)
	hit := h.LookupFresh(info, now+1000)
	if hit == nil || hit.Kind != CacheHit {
		t.Fatal("exact hit")
	}
	subset := ParseCall("call_capability", map[string]interface{}{
		"capability": "get_asset_blueprint",
		"arguments":  map[string]interface{}{"assetPath": "/Game/BP", "sections": []interface{}{"defaults"}},
	})
	sec := h.LookupFresh(subset, now+1000)
	if sec == nil || sec.Kind != CacheSectionHit {
		t.Fatalf("section hit %+v", sec)
	}
	wrapped := WrapDegraded(payload, "t")
	b, _ := jsonMarshal(wrapped)
	if !contains(string(b), "unavailable") {
		t.Fatal("degraded meta")
	}
}

func jsonMarshal(v interface{}) ([]byte, error) {
	return []byte(mustJSON(v)), nil
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()))
}
