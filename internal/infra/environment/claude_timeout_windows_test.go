//go:build windows

package environment

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeProbeTimeoutKillsChildTree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slow-claude.cmd")
	script := "@echo off\r\nping -n 8 127.0.0.1 >nul\r\n"
	if err := os.WriteFile(path, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := runClaudeVersion(ctx, path)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("timeout was not bounded: %v", elapsed)
	}
}
