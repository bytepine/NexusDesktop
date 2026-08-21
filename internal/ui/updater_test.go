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
	wantMac := "https://github.com/bytepine/NexusDesktop/releases/download/nexus-desktop-v1.2.3/NexusDesktop-darwin-universal-v1.2.3-update.zip"
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
		"def456 *NexusDesktop-darwin-universal-v1.2.3-update.zip\n"
	got, err := parseSHA256SUMS(body, "NexusDesktop-windows-amd64-v1.2.3-update.zip")
	if err != nil || got != "abc123" {
		t.Fatalf("windows zip: got %q err %v", got, err)
	}
	got, err = parseSHA256SUMS(body, "NexusDesktop-darwin-universal-v1.2.3-update.zip")
	if err != nil || got != "def456" {
		t.Fatalf("darwin zip: got %q err %v", got, err)
	}
}
