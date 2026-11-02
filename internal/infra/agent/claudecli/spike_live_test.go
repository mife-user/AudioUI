package claudecli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	claudecode "github.com/severity1/claude-agent-sdk-go"
)

func TestLiveSDKSpike(t *testing.T) {
	if os.Getenv("AUDIOUI_LIVE_CLI") != "1" {
		t.Skip("set AUDIOUI_LIVE_CLI=1 to run the live Claude CLI spike")
	}
	native := os.Getenv("AUDIOUI_CLAUDE_PATH")
	if native == "" {
		native = `D:\命令行工具\NOde-js\node_global\node_modules\@anthropic-ai\claude-code\bin\claude.exe`
	}
	if _, err := os.Stat(native); err != nil {
		t.Fatalf("native claude.exe not found at %s: %v", native, err)
	}

	workspace := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	permissionSeen := make(chan string, 4)
	client := claudecode.NewClient(
		claudecode.WithCLIPath(native),
		claudecode.WithCwd(workspace),
		claudecode.WithSettingSources(claudecode.SettingSourceUser),
		claudecode.WithPermissionMode(claudecode.PermissionModeDefault),
		claudecode.WithTools("Read", "Write"),
		claudecode.WithIncludePartialMessages(true),
		claudecode.WithStderrCallback(func(line string) { t.Logf("cli stderr: %s", line) }),
		claudecode.WithCanUseTool(func(_ context.Context, tool string, input map[string]any, _ claudecode.ToolPermissionContext) (claudecode.PermissionResult, error) {
			permissionSeen <- tool
			t.Logf("permission: %s input=%v", tool, input)
			return claudecode.NewPermissionResultDeny("spike denial"), nil
		}),
	)
	if err := client.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = client.Disconnect() }()

	if err := client.Query(ctx, "Create a file named spike.txt containing hello in the current working directory. Do not do anything else."); err != nil {
		t.Fatalf("query: %v", err)
	}

	msgCh := client.ReceiveMessages(ctx)
	seen := 0
	for {
		select {
		case msg := <-msgCh:
			if msg == nil {
				t.Logf("stream ended: %v", client.Err())
				if seen == 0 {
					t.Fatal("no messages received")
				}
				goto done
			}
			seen++
			switch m := msg.(type) {
			case *claudecode.SystemMessage:
				t.Logf("system: %T %+v", m, m)
			case *claudecode.AssistantMessage:
				t.Logf("assistant: %+v", m)
			case *claudecode.ResultMessage:
				t.Logf("result: isError=%v duration=%dms", m.IsError, m.DurationMs)
				goto done
			default:
				t.Logf("message: %T", m)
			}
		case <-ctx.Done():
			t.Fatalf("timeout: %v", ctx.Err())
		}
	}
done:
	select {
	case tool := <-permissionSeen:
		t.Logf("permission callback observed for %s", tool)
	default:
		t.Log("permission callback was not observed (agent may not have attempted Write)")
	}
	entries, _ := os.ReadDir(workspace)
	for _, entry := range entries {
		t.Logf("workspace entry: %s", entry.Name())
	}
	_ = filepath.Walk(workspace, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			t.Logf("workspace: %s", path)
		}
		return nil
	})
	fmt.Println("spike complete")
}
