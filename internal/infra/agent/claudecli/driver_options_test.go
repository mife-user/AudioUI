package claudecli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"audio-ui/internal/domain/port"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

// The agent must see the user profile settings so credentials from
// ~/.claude/settings.json (provider switchers, apiKeyHelper) reach the CLI.
// Project and local settings stay out of the session.
func TestOptionsLoadUserSettingsOnly(t *testing.T) {
	driver := NewDriver(NativeCLI{Path: `C:\fake\claude.exe`, Version: "2.1.224"}, "", nil)
	options, err := driver.options(port.DriverSpec{}, nil)
	if err != nil {
		t.Fatalf("options: %v", err)
	}

	var applied claudecode.Options
	for _, option := range options {
		option(&applied)
	}

	if len(applied.SettingSources) != 1 || applied.SettingSources[0] != claudecode.SettingSourceUser {
		t.Fatalf("setting sources = %v, want [%s]", applied.SettingSources, claudecode.SettingSourceUser)
	}
}

func TestValidateMCPConfigRequiresReadableValidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := validateMCPConfig(path); err == nil || !strings.HasPrefix(err.Error(), "MCP_CONFIG_NOT_FOUND:") {
		t.Fatalf("missing config error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers":`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateMCPConfig(path); err == nil || !strings.HasPrefix(err.Error(), "MCP_CONFIG_INVALID:") {
		t.Fatalf("invalid config error = %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"audioui":{"type":"http","url":"http://127.0.0.1:1/mcp"}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateMCPConfig(path); err != nil {
		t.Fatalf("valid config error = %v", err)
	}
}

func TestAgentStartErrorPreservesMCPConfigFailure(t *testing.T) {
	path := `C:\data\agent\p_test\r_test\mcp.json`
	for _, test := range []struct {
		stderr string
		code   string
	}{
		{"MCP config file not found: " + path, "MCP_CONFIG_NOT_FOUND"},
		{"MCP_CONFIG_UNREADABLE: " + path, "MCP_CONFIG_UNREADABLE"},
		{"Invalid MCP configuration", "MCP_CONFIG_INVALID"},
	} {
		diagnostics := &startupDiagnostics{}
		diagnostics.observeStderr(test.stderr)
		if err := agentStartError(errors.New("failed to initialize control protocol"), path, diagnostics); !strings.HasPrefix(err.Error(), test.code+":") {
			t.Fatalf("MCP start error = %v, want %s", err, test.code)
		}
	}
	if err := agentStartError(errors.New("CLI exited"), path, nil); !strings.HasPrefix(err.Error(), "AGENT_START_FAILED:") {
		t.Fatalf("generic start error = %v", err)
	}
}
