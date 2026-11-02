package claudecli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	mcpserver "audio-ui/internal/infra/agent/mcp"
)

type allowGate struct{ questionSeen chan string }

func (g *allowGate) Request(context.Context, valueobject.PermissionRequest) (valueobject.PermissionDecision, error) {
	return valueobject.DecisionAllow, nil
}
func (g *allowGate) Check(context.Context, string, map[string]any) port.PolicyResult {
	return port.PolicyResult{Decision: port.PolicyAllow, Tier: valueobject.PermissionAutoAllow, Reason: "spike"}
}
func (g *allowGate) ResolvePermission(string, valueobject.PermissionDecision) error { return nil }
func (g *allowGate) Ask(_ context.Context, request valueobject.QuestionRequest) (valueobject.QuestionAnswer, error) {
	g.questionSeen <- request.Question
	return valueobject.QuestionAnswer{Answer: "174", Via: "option"}, nil
}
func (g *allowGate) ResolveQuestion(string, valueobject.QuestionAnswer) error { return nil }

func TestLiveHostMCPAskUser(t *testing.T) {
	if os.Getenv("AUDIOUI_LIVE_CLI") != "1" {
		t.Skip("set AUDIOUI_LIVE_CLI=1")
	}
	native := os.Getenv("AUDIOUI_CLAUDE_PATH")
	if native == "" {
		native = `D:\命令行工具\NOde-js\node_global\node_modules\@anthropic-ai\claude-code\bin\claude.exe`
	}
	host, err := filepath.Abs("../../../../bin/audio-ui.exe")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	workspace := filepath.Join(root, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	gate := &allowGate{questionSeen: make(chan string, 1)}
	server, err := mcpserver.New(mcpserver.Options{
		Root: root, Workspace: workspace, ChainID: valueobject.DefaultChainID, DisplayID: "spike",
		QuestionGate: gate,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer server.Stop(context.Background())
	configPath := filepath.Join(root, "mcp.json")
	if err := server.WriteConfig(configPath); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var messages []string
	sink := func(event valueobject.AgentEvent) {
		mu.Lock()
		defer mu.Unlock()
		switch event.Type {
		case valueobject.EventMessage:
			if event.Message != nil {
				messages = append(messages, event.Message.Text)
			}
		case valueobject.EventTool:
			if event.Tool != nil {
				t.Logf("tool %s %s %s", event.Tool.ToolName, event.Tool.ToolStatus, event.Tool.ToolSummary)
			}
		case valueobject.EventError:
			if event.Error != nil {
				t.Logf("error %s %s", event.Error.Code, event.Error.Message)
			}
		case valueobject.EventStatus:
			if event.State != nil {
				t.Logf("status %s", event.State.Status)
			}
		}
	}
	driver := NewDriver(NativeCLI{Path: native, Version: "2.1.224"}, host, t.Logf)
	session, err := driver.Start(ctx, port.DriverSpec{
		ChainID: valueobject.DefaultChainID, Workspace: workspace,
		MCPConfigPath: configPath, MCPURL: server.URL(), MCPToken: server.Token(),
		EventSink: sink, PermissionGate: gate, QuestionGate: gate, ToolPolicy: gate,
		OnResumeRef: func(ref json.RawMessage) { t.Logf("resume ref: %s", string(ref)) },
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer session.Close()
	if err := session.Send(ctx, "Use the ask_user MCP tool exactly once to ask which tempo to use, options 174 and 87. Then tell me the chosen answer in one short sentence."); err != nil {
		t.Fatal(err)
	}
	select {
	case question := <-gate.questionSeen:
		t.Logf("question received: %s", question)
	case <-time.After(90 * time.Second):
		t.Fatal("ask_user was not called")
	}
	time.Sleep(5 * time.Second)
	mu.Lock()
	t.Logf("messages: %v", messages)
	mu.Unlock()
}
