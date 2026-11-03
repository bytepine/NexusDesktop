// Copyright byteyang. All Rights Reserved.
package proxy

import (
	"fmt"
	"testing"
)

func TestZeroTTLDisablesEveryCachePath(t *testing.T) {
	for _, format := range []string{"text", "structured", "legacy", "root"} {
		t.Run(format, func(t *testing.T) {
			h := NewHub()
			read := ParseCall("GetAssetBlueprint", map[string]interface{}{"AssetPath": "/synthetic/BP", "Sections": []interface{}{"All"}, "targetPort": float64(45000)})
			subset := ParseCall("GetAssetBlueprint", map[string]interface{}{"AssetPath": "/synthetic/BP", "Sections": []interface{}{"Defaults"}, "targetPort": float64(45000)})
			other := ParseCall("GetAssetBlueprint", map[string]interface{}{"AssetPath": "/synthetic/BP", "targetPort": float64(45001)})
			cached := map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "text", "text": `{"TtlSeconds":30}`}}}
			h.Store(read, cached, 1000)
			h.Store(subset, cached, 1000)
			h.Store(other, cached, 1000)
			payload := map[string]interface{}{"TtlSeconds": float64(0)}
			switch format {
			case "text", "legacy":
				key := "TtlSeconds"
				if format == "legacy" {
					key = "_ttl_seconds"
				}
				payload = map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "text", "text": mustJSON(map[string]interface{}{key: 0})}}}
			case "structured":
				payload = map[string]interface{}{"structuredContent": payload}
			}
			if ttl, _ := ExtractTtlMS(payload, read.Capability, 1000); ttl != 0 {
				t.Errorf("zero TTL parsed as %d", ttl)
			}
			h.Store(subset, payload, 1000)
			for _, query := range []CallInfo{read, subset} {
				if h.LookupFresh(query, 1000) != nil || h.LookupFresh(query, 1001) != nil || h.LookupDegraded(query, 100000) != nil {
					t.Error("zero TTL must remove prior exact/section/degraded snapshots and must not cache at the same timestamp")
				}
			}
			if h.LookupFresh(other, 1001) == nil {
				t.Error("zero TTL crossed instances")
			}
		})
	}
}

func TestProjectWriteInvalidatesOnlyItsInstance(t *testing.T) {
	for _, failed := range []bool{false, true} {
		h := NewHub()
		read := ParseCall("GetGameplayTags", map[string]interface{}{"Sections": []interface{}{"Hierarchy"}, "targetPort": float64(45000)})
		asset := ParseCall("GetAssetBlueprint", map[string]interface{}{"AssetPath": "/synthetic/BP", "targetPort": float64(45000)})
		other := ParseCall("GetGameplayTags", map[string]interface{}{"targetPort": float64(45001)})
		for _, query := range []CallInfo{read, asset, other} {
			h.Store(query, map[string]interface{}{"content": []interface{}{}}, 1000)
		}
		write := ParseCall("ManageGameplayTags", map[string]interface{}{"Tag": "Synthetic.Tag", "Operations": []interface{}{map[string]interface{}{"Action": "Remove"}}, "targetPort": float64(45000)})
		h.Store(write, map[string]interface{}{"isError": failed}, 1001)
		for _, query := range []CallInfo{read, asset} {
			if h.LookupFresh(query, 1002) != nil || h.LookupDegraded(query, 100000) != nil {
				t.Errorf("project write retained stale cache (partial failure=%v)", failed)
			}
		}
		if h.LookupFresh(other, 1002) == nil {
			t.Fatal("project write crossed instances")
		}
	}
}

func TestAsyncEditorWriteCache(t *testing.T) {
	for _, format := range []string{"text", "structured", "root"} {
		for _, terminal := range []string{"Completed", "Failed", "Cancelled", "Interrupted"} {
			t.Run(format+"/"+terminal, func(t *testing.T) {
				h := NewHub()
				read := ParseCall("GetAssetMaterial", map[string]interface{}{"AssetPath": "/synthetic/M", "targetPort": float64(45000)})
				other := ParseCall("GetAssetMaterial", map[string]interface{}{"AssetPath": "/synthetic/M", "targetPort": float64(45001)})
				write := ParseCall("ControlEditorNiagara", map[string]interface{}{"targetPort": float64(45000)})
				poll := ParseCall("GetEditorOperation", map[string]interface{}{"targetPort": float64(45000), "OperationId": "a", "WaitSeconds": 30})
				payload := func(source map[string]interface{}) map[string]interface{} {
					failed := source["State"] == "Failed"
					switch format {
					case "structured":
						return map[string]interface{}{"isError": failed, "structuredContent": source}
					case "text":
						return map[string]interface{}{"isError": failed, "content": []interface{}{map[string]interface{}{"type": "text", "text": mustJSON(source)}}}
					default:
						source["isError"] = failed
						return source
					}
				}
				operation := func(id, state string) map[string]interface{} {
					return payload(map[string]interface{}{"OperationId": id, "State": state, "TtlSeconds": 0})
				}
				for _, query := range []CallInfo{read, other} {
					h.Store(query, payload(map[string]interface{}{"Value": "before"}), 1000)
				}
				h.Store(write, operation("a", "Running"), 1001)
				h.Store(write, operation("b", "Running"), 1001)
				inFlightGeneration := h.CacheGeneration()
				h.Store(read, payload(map[string]interface{}{"Value": "intermediate"}), 1002)
				if h.LookupFresh(read, 1003) != nil || h.LookupDegraded(read, 100000) != nil {
					t.Fatal("running writes must suppress snapshots before polling")
				}
				if h.LookupFresh(other, 1003) == nil {
					t.Fatal("pending writes crossed instances")
				}
				h.Store(poll, payload(map[string]interface{}{"OperationId": "a", "State": "Running", "bWaitCancelled": true}), 1003)
				h.Store(poll, operation("a", terminal), 1004)
				h.Store(read, payload(map[string]interface{}{"Value": "intermediate"}), 1005)
				h.Store(ParseCall("GetEditorOperation", map[string]interface{}{"targetPort": float64(45001)}), operation("b", terminal), 1006)
				if h.LookupFresh(read, 1007) != nil {
					t.Fatal("another task or instance released pending writes")
				}
				h.Store(poll, payload(map[string]interface{}{"Operations": []interface{}{map[string]interface{}{"OperationId": "b"}}, "Operations_defaults": map[string]interface{}{"State": terminal}, "TtlSeconds": 0}), 1008)
				if h.LookupFresh(read, 1009) != nil {
					t.Fatal("terminal/list replies retained intermediate snapshots")
				}
				h.Store(read, payload(map[string]interface{}{"Value": "late intermediate"}), 1002, inFlightGeneration)
				if h.LookupFresh(read, 1009) != nil {
					t.Fatal("a read issued before completion refilled caches after the terminal reply")
				}
				h.Store(read, payload(map[string]interface{}{"Value": "after"}), 1010)
				if h.LookupFresh(read, 1011) == nil {
					t.Fatal("last terminal task did not restore caching")
				}
				h.Store(write, operation("waited", terminal), 1012)
				if h.LookupFresh(read, 1013) != nil {
					t.Fatal("write completing inside its initial wait retained snapshots")
				}
				h.Store(read, payload(map[string]interface{}{"Value": "after-wait"}), 1014)
				if h.LookupFresh(read, 1015) == nil {
					t.Fatal("completed write unnecessarily blocked caching")
				}
			})
		}
	}
	h := NewHub()
	read := ParseCall("GetAssetMaterial", map[string]interface{}{"AssetPath": "/synthetic/M", "targetPort": float64(45000)})
	other := ParseCall("GetAssetMaterial", map[string]interface{}{"AssetPath": "/synthetic/M", "targetPort": float64(45001)})
	write := ParseCall("InteractEditorSlate", map[string]interface{}{"targetPort": float64(45000)})
	for i := 0; i < 129; i++ {
		h.Store(write, map[string]interface{}{"OperationId": fmt.Sprint(i), "State": "Running"}, 1000)
	}
	for i := 0; i < 129; i++ {
		h.Store(ParseCall("GetEditorOperation", map[string]interface{}{"targetPort": float64(45000)}), map[string]interface{}{"OperationId": fmt.Sprint(i), "State": "Completed"}, 1001)
	}
	h.Store(read, map[string]interface{}{"Value": "unknown"}, 1002)
	h.Store(other, map[string]interface{}{"Value": "other"}, 1002)
	if h.LookupFresh(read, 1003) != nil || h.LookupFresh(other, 1003) == nil {
		t.Fatal("per-instance overflow must stay conservative and isolated")
	}
	for i := 1; i <= 64; i++ {
		h.Store(ParseCall("InteractEditorSlate", map[string]interface{}{"targetPort": float64(45000 + i)}), map[string]interface{}{"OperationId": "pending", "State": "Running"}, 1004)
	}
	h.Store(other, map[string]interface{}{"Value": "unknown"}, 1005)
	if h.LookupFresh(other, 1006) != nil {
		t.Fatal("global tracking overflow must fail closed")
	}
}
