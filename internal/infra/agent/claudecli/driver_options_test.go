package claudecli

import (
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
