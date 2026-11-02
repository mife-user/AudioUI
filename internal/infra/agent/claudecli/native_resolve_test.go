package claudecli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestShimCandidatesResolveNpmLayout(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "claude.cmd")
	content := "@ECHO off\r\nGOTO start\r\n:start\r\n\"%dp0%\\node_modules\\@anthropic-ai\\claude-code\\bin\\claude.exe\" %*\r\n"
	if err := os.WriteFile(shim, []byte(content), 0o600); err != nil {
		t.Fatalf("write shim: %v", err)
	}

	want := filepath.Join(dir, "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe")
	if !slices.Contains(shimCandidates(shim), want) {
		t.Fatalf("shimCandidates(%q) missing %q: %v", shim, want, shimCandidates(shim))
	}
}

func TestShimCandidatesKeepAbsoluteExe(t *testing.T) {
	dir := t.TempDir()
	shim := filepath.Join(dir, "claude.cmd")
	if err := os.WriteFile(shim, []byte("\"C:\\Tools\\claude\\claude.exe\" %*\r\n"), 0o600); err != nil {
		t.Fatalf("write shim: %v", err)
	}

	if !slices.Contains(shimCandidates(shim), `C:\Tools\claude\claude.exe`) {
		t.Fatalf("shimCandidates(%q) missing absolute exe: %v", shim, shimCandidates(shim))
	}
}

func TestResolveNativeRejectsBatchShim(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.MkdirAll(empty, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	shim := filepath.Join(dir, "claude.cmd")
	if err := os.WriteFile(shim, []byte("@ECHO off\r\n"), 0o600); err != nil {
		t.Fatalf("write shim: %v", err)
	}

	// Isolate the lookup so only the explicit shim is a candidate.
	t.Setenv("PATH", empty)
	t.Setenv("USERPROFILE", empty)
	t.Setenv("HOME", empty)

	_, err := ResolveNative(context.Background(), shim)
	if !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("ResolveNative(%q) err = %v, want ErrAgentUnavailable", shim, err)
	}
}
