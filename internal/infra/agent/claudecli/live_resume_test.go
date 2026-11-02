package claudecli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	mcpserver "audio-ui/internal/infra/agent/mcp"
)

type captureGate struct{}

func (captureGate) Request(context.Context, valueobject.PermissionRequest) (valueobject.PermissionDecision, error) {
	return valueobject.DecisionAllow, nil
}
func (captureGate) Check(context.Context, string, map[string]any) port.PolicyResult {
	return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow}
}
func (captureGate) ResolvePermission(string, valueobject.PermissionDecision) error { return nil }
func (captureGate) Ask(context.Context, valueobject.QuestionRequest) (valueobject.QuestionAnswer, error) {
	return valueobject.QuestionAnswer{Answer: "A"}, nil
}
func (captureGate) ResolveQuestion(string, valueobject.QuestionAnswer) error { return nil }

func startMCP(t *testing.T, root, workspace string) (*mcpserver.Server, string) {
	t.Helper()
	server, err := mcpserver.New(mcpserver.Options{Root: root, Workspace: workspace, ChainID: valueobject.DefaultChainID, DisplayID: "r", QuestionGate: captureGate{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(root, "mcp.json")
	if err := server.WriteConfig(config); err != nil {
		t.Fatal(err)
	}
	return server, config
}

func TestLiveResumeWithNewMCPToken(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	driver := NewDriver(NativeCLI{Path: native, Version: "2.1.224"}, host, t.Logf)

	var ref json.RawMessage
	var mu sync.Mutex
	var output strings.Builder
	sink := func(event valueobject.AgentEvent) {
		if event.Type == valueobject.EventMessage && event.Message != nil {
			mu.Lock()
			output.WriteString(event.Message.Text)
			mu.Unlock()
		}
		if event.Type == valueobject.EventStatus && event.State != nil {
			t.Logf("status=%s", event.State.Status)
		}
	}
	onRef := func(value json.RawMessage) { ref = append(json.RawMessage{}, value...) }

	server1, config1 := startMCP(t, root, workspace)
	session1, err := driver.Start(ctx, port.DriverSpec{ChainID: valueobject.DefaultChainID, Workspace: workspace, MCPConfigPath: config1, MCPURL: server1.URL(), MCPToken: server1.Token(), EventSink: sink, OnResumeRef: onRef, PermissionGate: captureGate{}, QuestionGate: captureGate{}, ToolPolicy: captureGate{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := session1.Send(ctx, "Remember the single codeword ORANGE. Reply only OK."); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, &mu, &output)
	_ = session1.Close()
	_ = server1.Stop(context.Background())
	if len(ref) == 0 {
		t.Fatal("no resume ref")
	}
	t.Logf("resume ref: %s", string(ref))

	server2, config2 := startMCP(t, root, workspace)
	defer server2.Stop(context.Background())
	output.Reset()
	mu.Lock()
	output.Reset()
	mu.Unlock()
	session2, err := driver.Resume(ctx, port.DriverSpec{ChainID: valueobject.DefaultChainID, Workspace: workspace, MCPConfigPath: config2, MCPURL: server2.URL(), MCPToken: server2.Token(), EventSink: sink, OnResumeRef: onRef, PermissionGate: captureGate{}, QuestionGate: captureGate{}, ToolPolicy: captureGate{}}, ref)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	defer session2.Close()
	if err := session2.Send(ctx, "What was the codeword? Reply with only the codeword."); err != nil {
		t.Fatal(err)
	}
	waitIdle(t, &mu, &output)
	mu.Lock()
	text := output.String()
	mu.Unlock()
	t.Logf("resumed output: %s", text)
	if !strings.Contains(strings.ToUpper(text), "ORANGE") {
		t.Fatalf("resume context not retained: %q", text)
	}
}

func waitIdle(t *testing.T, mu *sync.Mutex, output *strings.Builder) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		mu.Lock()
		n := output.Len()
		mu.Unlock()
		if n > 0 {
			time.Sleep(3 * time.Second)
			return
		}
	}
	t.Fatal("timeout waiting for output")
}
