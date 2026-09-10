// Copyright byteyang. All Rights Reserved.

package ui

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0.5", "1.0.4", true},
		{"1.0.4", "1.0.5", false},
		{"1.0.5", "1.0.5", false},
		{"1.1.0", "1.0.9", true},
		{"2.0.0", "1.99.99", true},
		{"1.0.1", "1.0", true},
		{"1.0", "1.0.0", false},
		{"1.0.6-beta.1", "1.0.5", true},
		{"v1.0.6", "1.0.5", true},
		// 同主段：正式版新于预发布版，反向不成立
		{"2.0.0", "2.0.0-beta.3", true},
		{"2.0.0-beta.3", "2.0.0", false},
		{"2.0.0-beta.4", "2.0.0-beta.3", true},
		{"2.0.0-beta.3", "2.0.0-beta.4", false},
		{"2.0.0-beta.3", "2.0.0-beta.3", false},
		{"2.0.0-beta.10", "2.0.0-beta.9", true},
		{"2.0.0-beta.9", "2.0.0-beta.10", false},
		{"2.0.0-rc.1", "2.0.0-beta.9", true},
		{"2.0.0", "2.0.0+build", false},
		{"2.0.0+build", "2.0.0", false},
		{"2.0.0-beta.1", "2.0.0+build", false},
	}
	for _, c := range cases {
		if got := IsNewerVersion(c.a, c.b); got != c.want {
			t.Errorf("IsNewerVersion(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestParseLatestTagFromURL(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://github.com/bytepine/NexusDesktop/releases/tag/nexus-desktop-v1.0.5", "1.0.5"},
		{"https://github.com/bytepine/NexusDesktop/releases/tag/nexus-desktop-v1.0.5?foo=1", "1.0.5"},
		{"https://github.com/bytepine/NexusDesktop/releases/tag/nexus-desktop-v2.0.0-beta.3", "2.0.0-beta.3"},
		{"https://github.com/bytepine/NexusDesktop/releases/tag/v2.0.0", "2.0.0"},
		{"https://github.com/bytepine/NexusDesktop/releases", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := parseLatestTagFromURL(c.url); got != c.want {
			t.Errorf("parseLatestTagFromURL(%q) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestUpdateZipURLFor(t *testing.T) {
	win := updateZipURLFor("windows", "1.2.3")
	wantWin := "https://github.com/bytepine/NexusDesktop/releases/download/nexus-desktop-v1.2.3/NexusDesktop-windows-amd64-v1.2.3-update.zip"
	if win != wantWin {
		t.Errorf("windows url = %q, want %q", win, wantWin)
	}
	mac := updateZipURLFor("darwin", "1.2.3")
	wantMac := "https://github.com/bytepine/NexusDesktop/releases/download/nexus-desktop-v1.2.3/NexusDesktop-darwin-arm64-v1.2.3-update.zip"
	if mac != wantMac {
		t.Errorf("darwin url = %q, want %q", mac, wantMac)
	}
	if updateZipURLFor("linux", "1.2.3") != "" {
		t.Error("linux should have no in-place zip url")
	}
}

func TestGithubReleasePage(t *testing.T) {
	got := githubReleasePage("1.2.3")
	want := "https://github.com/bytepine/NexusDesktop/releases/tag/nexus-desktop-v1.2.3"
	if got != want {
		t.Errorf("githubReleasePage = %q, want %q", got, want)
	}
	if githubReleasePage("") != releasesURL {
		t.Error("empty version should open latest")
	}
}

func TestExtractZipAndFindExe(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "upd.zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(zf)
	fw, err := w.Create("NexusDesktop.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte("fake-exe")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	_ = zf.Close()

	out := filepath.Join(dir, "out")
	if err := extractZip(zipPath, out); err != nil {
		t.Fatal(err)
	}
	found, err := findNamedFile(out, "NexusDesktop.exe")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(found)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "fake-exe" {
		t.Fatalf("content = %q", data)
	}
}

func TestParseAtomTags(t *testing.T) {
	raw := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <entry>
    <title>nexus-desktop-v2.0.0-beta.3</title>
    <link rel="alternate" href="https://github.com/bytepine/NexusDesktop/releases/tag/nexus-desktop-v2.0.0-beta.3"/>
  </entry>
  <entry>
    <title>nexus-desktop-v1.1.1</title>
    <link rel="alternate" href="https://github.com/bytepine/NexusDesktop/releases/tag/nexus-desktop-v1.1.1"/>
  </entry>
</feed>`)
	got, err := parseAtomTags(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "2.0.0-beta.3" || got[1] != "1.1.1" {
		t.Fatalf("got %#v", got)
	}
}

func TestSelectUpdate(t *testing.T) {
	const (
		stable = "stable"
		pre    = "pre"
	)
	cases := []struct {
		current, resolved, latest string
		atom                      []string
		want                      string
	}{
		{"1.1.1", stable, "2.0.0", nil, "2.0.0"},
		{"1.1.1", pre, "2.0.0", nil, "2.0.0"},
		{"1.1.1", stable, "1.1.1", []string{"2.0.0-beta.3"}, ""},
		{"1.1.1", pre, "1.1.1", []string{"2.0.0-beta.3"}, "2.0.0-beta.3"},
		{"1.1.1", stable, "2.0.0", []string{"2.1.0-beta.1"}, "2.0.0"},
		{"1.1.1", pre, "2.0.0", []string{"2.1.0-beta.1"}, "2.1.0-beta.1"},
		{"2.0.0", stable, "2.0.0", []string{"2.0.0-beta.4"}, ""},
		{"2.0.0", pre, "2.0.0", []string{"2.0.0-beta.4"}, ""},
		{"2.0.0", pre, "2.0.0", []string{"2.1.0-beta.1"}, "2.1.0-beta.1"},
		{"2.0.0-beta.3", stable, "1.1.1", nil, ""},
		{"2.0.0-beta.3", pre, "1.1.1", nil, ""},
		{"2.0.0-beta.3", pre, "1.1.1", []string{"2.0.0-beta.4"}, "2.0.0-beta.4"},
		{"2.0.0-beta.3", stable, "2.0.0", nil, "2.0.0"},
		{"2.0.0-beta.3", pre, "2.0.0", nil, "2.0.0"},
		{"2.0.0-beta.3", stable, "2.0.0", []string{"2.0.0-beta.4"}, "2.0.0"},
		{"2.0.0-beta.3", pre, "2.0.0", []string{"2.0.0-beta.4"}, "2.0.0"},
		{"2.0.0-beta.3", pre, "", []string{"2.0.0-beta.4"}, "2.0.0-beta.4"},
		{"2.0.0-beta.3", stable, "2.0.0", []string{"2.1.0-beta.1"}, "2.0.0"},
		{"2.0.0-beta.3", pre, "2.0.0", []string{"2.1.0-beta.1"}, "2.1.0-beta.1"},
		// pre 并入 latest：atom 没有正式版时仍能 beta→正式
		{"2.0.0-beta.3", pre, "2.0.0", []string{"2.0.0-beta.3"}, "2.0.0"},
		{"2.0.0", stable, "2.0.0-beta.4", nil, ""},
	}
	for _, c := range cases {
		got := selectUpdate(c.current, c.resolved, c.latest, c.atom)
		if got != c.want {
			t.Errorf("selectUpdate(%q, %q, %q, %v) = %q, want %q",
				c.current, c.resolved, c.latest, c.atom, got, c.want)
		}
	}
}

func TestZipSlipRejected(t *testing.T) {
	if _, err := zipSlipSafe(t.TempDir(), "../evil.exe"); err == nil {
		t.Fatal("expected zip slip to be rejected")
	}
}

func TestSupportsInPlaceUpdate(t *testing.T) {
	if SupportsInPlaceUpdate("dev") || SupportsInPlaceUpdate("") {
		t.Fatal("dev build must not apply in-place update")
	}
}

func TestParseSHA256SUMS(t *testing.T) {
	body := "abc123  NexusDesktop-windows-amd64-v1.2.3-update.zip\n" +
		"def456 *NexusDesktop-darwin-arm64-v1.2.3-update.zip\n"
	got, err := parseSHA256SUMS(body, "NexusDesktop-windows-amd64-v1.2.3-update.zip")
	if err != nil || got != "abc123" {
		t.Fatalf("windows zip: got %q err %v", got, err)
	}
	got, err = parseSHA256SUMS(body, "NexusDesktop-darwin-arm64-v1.2.3-update.zip")
	if err != nil || got != "def456" {
		t.Fatalf("darwin zip: got %q err %v", got, err)
	}
}

func TestCleanupUpdateTempDir(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "other.txt")
	stale := filepath.Join(dir, "NexusDesktop-update-9.9.9")
	if err := os.WriteFile(keep, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(stale, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "NexusDesktop.exe"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	cleanupUpdateTempDir(dir)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale update dir should be removed, err=%v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unrelated file should remain: %v", err)
	}
}
