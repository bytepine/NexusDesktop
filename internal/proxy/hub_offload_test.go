// Copyright byteyang. All Rights Reserved.

package proxy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPurgeOffloadOnNewHub(t *testing.T) {
	dir := t.TempDir()
	offloadRoot = dir
	leftover := filepath.Join(dir, "leftover.json")
	if err := os.WriteFile(leftover, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = NewHub()
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Fatal("startup leftover remains")
	}
}

func TestOffloadWritesRestrictedFile(t *testing.T) {
	dir := t.TempDir()
	offloadRoot = dir
	h := NewHub()
	big := strings.Repeat("x", OffloadChars+1)
	info := ParseCall("get_asset_details", map[string]interface{}{"assetPath": "/Game/A"})
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
}

func TestPruneOffloadOlderThanHour(t *testing.T) {
	dir := t.TempDir()
	offloadRoot = dir
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
