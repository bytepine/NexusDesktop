// Copyright byteyang. All Rights Reserved.

package ui

import "testing"

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
