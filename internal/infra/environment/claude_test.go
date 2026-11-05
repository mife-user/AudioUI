package environment

import (
	"context"
	"errors"
	"testing"

	"audio-ui/internal/infra/agent/claudecli"
)

func TestClaudeProbeReportsUnavailableNativeCLI(t *testing.T) {
	probe := &ClaudeProbe{
		resolveNative: func(context.Context, string) (claudecli.NativeCLI, error) {
			return claudecli.NativeCLI{}, errors.New("native cli not found")
		},
	}
	status := probe.Detect(context.Background())
	if status.Installed {
		t.Fatal("Installed = true, want false")
	}
	if status.Error != "native cli not found" {
		t.Fatalf("Error = %q", status.Error)
	}
}

func TestClaudeProbeUsesRuntimeNativeResolver(t *testing.T) {
	t.Setenv("AUDIOUI_CLAUDE_PATH", `C:\tools\claude.exe`)
	probe := &ClaudeProbe{
		resolveNative: func(_ context.Context, explicit string) (claudecli.NativeCLI, error) {
			if explicit != `C:\tools\claude.exe` {
				t.Fatalf("explicit path = %q", explicit)
			}
			return claudecli.NativeCLI{Path: explicit, Version: "2.1.224"}, nil
		},
	}
	status := probe.Detect(context.Background())
	if !status.Installed || status.Path != `C:\tools\claude.exe` || status.Version != "2.1.224" || status.Error != "" {
		t.Fatalf("status = %+v", status)
	}
}
