//go:build windows

package agenthost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWithoutEnvDropsOnlyTargetKey(t *testing.T) {
	env := []string{"PATH=C:\\bin", "AUDIOUI_AGENT_HOST=1", "AUDIOUI_NATIVE_CLAUDE=C:\\claude.exe", "USERPROFILE=C:\\Users\\u"}

	got := withoutEnv(env, EnvMode)

	if len(got) != 3 {
		t.Fatalf("env = %v, want 3 entries", got)
	}
	for _, item := range got {
		if item == "AUDIOUI_AGENT_HOST=1" {
			t.Fatalf("dropped key still present: %v", got)
		}
	}
	seen := map[string]bool{}
	for _, item := range got {
		seen[item] = true
	}
	for _, want := range []string{"PATH=C:\\bin", "AUDIOUI_NATIVE_CLAUDE=C:\\claude.exe", "USERPROFILE=C:\\Users\\u"} {
		if !seen[want] {
			t.Fatalf("env lost %q: %v", want, got)
		}
	}
}

func TestWithoutEnvLeavesEmptyInput(t *testing.T) {
	if got := withoutEnv(nil, EnvMode); len(got) != 0 {
		t.Fatalf("env = %v, want empty", got)
	}
}

func TestRunWithoutNativePathFails(t *testing.T) {
	t.Setenv(EnvNative, "")
	if code := Run(nil); code != 2 {
		t.Fatalf("Run exit code = %d, want 2", code)
	}
}

func TestIsHostReadsModeEnv(t *testing.T) {
	t.Setenv(EnvMode, "1")
	if !IsHost() {
		t.Fatal("IsHost() = false with AUDIOUI_AGENT_HOST=1")
	}
	t.Setenv(EnvMode, "")
	if IsHost() {
		t.Fatal("IsHost() = true without AUDIOUI_AGENT_HOST")
	}
}

func TestMCPConfigArgumentAcceptsBothForms(t *testing.T) {
	for _, args := range [][]string{
		{"--mcp-config", `C:\data\mcp.json`},
		{`--mcp-config=C:\data\mcp.json`},
	} {
		path, ok := mcpConfigArgument(args)
		if !ok || path != `C:\data\mcp.json` {
			t.Fatalf("mcpConfigArgument(%v) = %q, %v", args, path, ok)
		}
	}
}

func TestValidateMCPConfigPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := validateMCPConfigPath(path); err == nil || !strings.HasPrefix(err.Error(), "MCP_CONFIG_NOT_FOUND:") {
		t.Fatalf("missing config error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateMCPConfigPath(path); err != nil {
		t.Fatalf("existing config error = %v", err)
	}
	if err := validateMCPConfigPath(t.TempDir()); err == nil || !strings.HasPrefix(err.Error(), "MCP_CONFIG_INVALID:") {
		t.Fatalf("directory config error = %v", err)
	}
}
