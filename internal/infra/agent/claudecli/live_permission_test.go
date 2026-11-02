package claudecli

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	mcpserver "audio-ui/internal/infra/agent/mcp"
)

type confirmGate struct {
	mu      sync.Mutex
	seen    []string
	allowed bool
}

func (g *confirmGate) Request(_ context.Context, req valueobject.PermissionRequest) (valueobject.PermissionDecision, error) {
	g.mu.Lock()
	g.seen = append(g.seen, req.Tool)
	g.mu.Unlock()
	if g.allowed {
		return valueobject.DecisionAllow, nil
	}
	return valueobject.DecisionDeny, nil
}
func (g *confirmGate) Check(context.Context, string, map[string]any) port.PolicyResult {
	return port.PolicyResult{Decision: port.PolicyConfirm, Tier: valueobject.PermissionConfirm, Reason: "confirm"}
}
func (g *confirmGate) ResolvePermission(string, valueobject.PermissionDecision) error { return nil }
func (g *confirmGate) Ask(context.Context, valueobject.QuestionRequest) (valueobject.QuestionAnswer, error) {
	return valueobject.QuestionAnswer{}, nil
}
func (g *confirmGate) ResolveQuestion(string, valueobject.QuestionAnswer) error { return nil }

func TestLiveWritePermission(t *testing.T) {
	if os.Getenv("AUDIOUI_LIVE_CLI") != "1" {
		t.Skip("set AUDIOUI_LIVE_CLI=1")
	}
	native := os.Getenv("AUDIOUI_CLAUDE_PATH")
	if native == "" {
		native = `D:\命令行工具\NOde-js\node_global\node_modules\@anthropic-ai\claude-code\bin\claude.exe`
	}
	host, _ := filepath.Abs("../../../../bin/audio-ui.exe")
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	_ = os.MkdirAll(workspace, 0o755)
	gate := &confirmGate{}
	server, err := mcpserver.New(mcpserver.Options{Root: root, Workspace: workspace, ChainID: valueobject.DefaultChainID, DisplayID: "p", QuestionGate: gate})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	configPath := filepath.Join(root, "mcp.json")
	if err := server.WriteConfig(configPath); err != nil {
		t.Fatal(err)
	}
	driver := NewDriver(NativeCLI{Path: native, Version: "2.1.224"}, host, t.Logf)
	session, err := driver.Start(ctx, port.DriverSpec{
		ChainID: valueobject.DefaultChainID, Workspace: workspace,
		MCPConfigPath: configPath, MCPURL: server.URL(), MCPToken: server.Token(),
		EventSink: func(event valueobject.AgentEvent) {
			if event.Type == valueobject.EventError {
				t.Logf("error event: %+v", event.Error)
			}
		},
		PermissionGate: gate, QuestionGate: gate, ToolPolicy: gate,
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()
	if err := session.Send(ctx, "Create a file named blocked.txt containing hello in the current working directory. Use the Write tool. Do not do anything else."); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		gate.mu.Lock()
		seen := append([]string{}, gate.seen...)
		gate.mu.Unlock()
		if len(seen) > 0 {
			if _, err := os.Stat(filepath.Join(workspace, "blocked.txt")); err == nil {
				t.Fatalf("file was written despite deny; permissions seen=%v", seen)
			}
			t.Logf("permission callback observed: %v", seen)
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("Write did not reach the permission callback")
}
