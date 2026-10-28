// Copyright byteyang. All Rights Reserved.

package proxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func isolatedOffloadRoot(t *testing.T) string {
	t.Helper()
	previous := offloadRoot
	dir := t.TempDir()
	offloadRoot = dir
	t.Cleanup(func() { offloadRoot = previous })
	return dir
}

func TestNewHubKeepsFreshOffload(t *testing.T) {
	dir := isolatedOffloadRoot(t)
	leftover := filepath.Join(dir, "leftover.json")
	if err := os.WriteFile(leftover, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = NewHub()
	if _, err := os.Stat(leftover); err != nil {
		t.Fatal("new hub deleted another hub's fresh file")
	}
}

func TestInlineImagePreservedWithLargeMetadata(t *testing.T) {
	isolatedOffloadRoot(t)
	h := NewHub()
	info := ParseCall("GetAssetBlueprint", map[string]interface{}{"AssetPath": "/Game/ImageProbe"})
	metadata := map[string]interface{}{"FilePath": "capture.png", "Padding": strings.Repeat("x", OffloadChars+1)}
	text, _ := json.Marshal(metadata)
	image := map[string]interface{}{"type": "image", "mimeType": "image/png", "data": "aW1hZ2U="}
	result := map[string]interface{}{
		"structuredContent": metadata,
		"content":           []interface{}{map[string]interface{}{"type": "text", "text": string(text)}, image},
	}
	stored := h.Store(info, result, time.Now().UnixMilli()).(map[string]interface{})
	content := stored["content"].([]interface{})
	if len(content) != 2 || content[1].(map[string]interface{})["data"] != image["data"] {
		t.Fatal("inline image was replaced by an offload summary")
	}
	if stored["structuredContent"].(map[string]interface{})["FilePath"] != "capture.png" {
		t.Fatal("structured image metadata was lost")
	}
	if h.LookupFresh(info, time.Now().UnixMilli()) != nil {
		t.Fatal("image result must not enter snapshot cache")
	}
}

func TestLegacyCleanupDoesNotTouchOffload(t *testing.T) {
	dir := t.TempDir()
	previous := offloadRoot
	offloadRoot = filepath.Join(dir, "nexus-mcp-offload-v2")
	t.Cleanup(func() { offloadRoot = previous })
	h := NewHub()
	stored := h.offload(map[string]interface{}{"content": []interface{}{map[string]interface{}{"type": "text", "text": "payload"}}}).(map[string]interface{})
	var summary map[string]interface{}
	text := stored["content"].([]interface{})[0].(map[string]interface{})["text"].(string)
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		t.Fatal(err)
	}
	legacyRoot := filepath.Join(dir, "nexus-mcp-offload")
	if err := os.MkdirAll(legacyRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "old.json"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(legacyRoot)
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(legacyRoot, entry.Name())); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(summary["path"].(string)); err != nil {
		t.Fatal("legacy 2.x cleanup deleted a new payload", err)
	}
}

func TestOffloadWritesRestrictedFile(t *testing.T) {
	isolatedOffloadRoot(t)
	h := NewHub()
	big := strings.Repeat("x", OffloadChars+1)
	info := ParseCall("GetAssetBlueprint", map[string]interface{}{"AssetPath": "/Game/A"})
	result := map[string]interface{}{
		"content": []interface{}{map[string]interface{}{"type": "text", "text": big}},
	}
	stored := h.Store(info, result, time.Now().UnixMilli())
	rec, _ := stored.(map[string]interface{})
	content, _ := rec["content"].([]interface{})
	if len(content) == 0 {
		t.Fatalf("no content: %#v", stored)
	}
	first, _ := content[0].(map[string]interface{})
	text, _ := first["text"].(string)
	if !strings.Contains(text, `"offloaded":true`) {
		t.Fatalf("not offloaded: %s", text)
	}
	if !strings.Contains(text, "offload-") {
		t.Fatalf("path missing: %s", text)
	}
	var summary map[string]interface{}
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		t.Fatal(err)
	}
	file := summary["path"].(string)
	if _, err := os.Stat(file); err != nil {
		t.Fatal("offload file missing", err)
	}
	other := NewHub()
	otherStored := other.Store(info, result, time.Now().UnixMilli()).(map[string]interface{})
	otherText := otherStored["content"].([]interface{})[0].(map[string]interface{})["text"].(string)
	if err := json.Unmarshal([]byte(otherText), &summary); err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(summary["path"].(string)) == filepath.Dir(file) {
		t.Fatal("hubs must use separate offload directories")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatal("another hub removed active payload", err)
	}
	// 落盘失败时必须返回原始内容，不能伪造成功的文件引用。
	h.offloadDir = file
	if got := h.Store(info, result, time.Now().UnixMilli()); ContentTextLen(got) != len(big) {
		t.Fatal("failed offload discarded original response")
	}
}

func TestPruneOffloadOlderThanHour(t *testing.T) {
	dir := isolatedOffloadRoot(t)
	dir = filepath.Join(dir, "old-hub")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(dir, "stale.json")
	fresh := filepath.Join(dir, "fresh.json")
	if err := os.WriteFile(stale, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fresh, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}
	pruneOffload(time.Hour)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale remains")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatal("fresh removed")
	}
}
