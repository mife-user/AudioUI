//go:build windows

package environment

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidWebView2Version(t *testing.T) {
	for _, version := range []string{"156.0.4314.50", "100.0.0.0"} {
		if !validWebView2Version(version) {
			t.Fatalf("%q should be valid", version)
		}
	}
	for _, version := range []string{"", " ", "0.0.0.0"} {
		if validWebView2Version(version) {
			t.Fatalf("%q should be invalid", version)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	if compareVersions("100.0.1185.0", "99.0.1150.0") <= 0 {
		t.Fatal("100.x should sort after 99.x")
	}
	if compareVersions("156.0.4314.50", "156.0.4314.50") != 0 {
		t.Fatal("equal versions should compare equal")
	}
}

func TestNewestWebView2Version(t *testing.T) {
	root := t.TempDir()
	for _, version := range []string{"99.0.1150.0", "100.0.1185.0", "not-a-version"} {
		dir := filepath.Join(root, version)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if version != "not-a-version" {
			if err := os.WriteFile(filepath.Join(dir, webView2ExeName), []byte("stub"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := newestWebView2Version(root); got != "100.0.1185.0" {
		t.Fatalf("newest = %q", got)
	}
}
