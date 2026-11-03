// Copyright byteyang. All Rights Reserved.
package proxy

import "testing"

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
