package environment

import (
	"context"
	"errors"
	"testing"
)

func TestClaudeProbeMissing(t *testing.T) {
	probe := &ClaudeProbe{
		LookPath: func(string) (string, error) { return "", errors.New("not found") },
		Run:      func(context.Context, string) ([]byte, error) { return nil, nil },
	}
	status := probe.Detect(context.Background())
	if status.Installed {
		t.Fatal("Installed = true, want false")
	}
	if status.Error == "" {
		t.Fatal("Error should explain the missing CLI")
	}
}

func TestClaudeProbeSuccess(t *testing.T) {
	probe := &ClaudeProbe{
		LookPath: func(string) (string, error) { return `C:\tools\claude.cmd`, nil },
		Run: func(context.Context, string) ([]byte, error) {
			return []byte("\x1b[32m2.1.224 (Claude Code)\x1b[0m\r\n"), nil
		},
	}
	status := probe.Detect(context.Background())
	if !status.Installed {
		t.Fatal("Installed = false, want true")
	}
	if status.Version != "2.1.224 (Claude Code)" {
		t.Fatalf("Version = %q", status.Version)
	}
	if status.Path != `C:\tools\claude.cmd` {
		t.Fatalf("Path = %q", status.Path)
	}
}

func TestClaudeProbeVersionFailureIsNotFatal(t *testing.T) {
	probe := &ClaudeProbe{
		LookPath: func(string) (string, error) { return `C:\tools\claude.exe`, nil },
		Run:      func(context.Context, string) ([]byte, error) { return nil, errors.New("exit 1") },
	}
	status := probe.Detect(context.Background())
	if !status.Installed {
		t.Fatal("Installed = false, want true")
	}
	if status.Version != "已安装" {
		t.Fatalf("Version = %q", status.Version)
	}
	if status.Error == "" {
		t.Fatal("Error should be preserved")
	}
}

func TestClaudeCommandUsesCallForBatchFiles(t *testing.T) {
	cmd := claudeCommand(context.Background(), `C:\tools\claude.cmd`)
	args := cmd.Args
	if len(args) < 6 || args[0] != "cmd.exe" || args[4] != "call" {
		t.Fatalf("unexpected batch command: %#v", args)
	}
	if args[5] != `C:\tools\claude.cmd` {
		t.Fatalf("path arg = %q", args[5])
	}
}

func TestClaudeCommandExecutesExeDirectly(t *testing.T) {
	cmd := claudeCommand(context.Background(), `C:\tools\claude.exe`)
	if len(cmd.Args) < 2 || cmd.Args[0] != `C:\tools\claude.exe` {
		t.Fatalf("unexpected exe command: %#v", cmd.Args)
	}
}

func TestFirstNonEmptyLine(t *testing.T) {
	got := firstNonEmptyLine("\r\n\x1b[31merror text\x1b[0m\r\n2.1.224\r\n")
	if got != "error text" {
		t.Fatalf("got %q", got)
	}
	if got := firstNonEmptyLine(" \r\n"); got != "" {
		t.Fatalf("empty output got %q", got)
	}
}
