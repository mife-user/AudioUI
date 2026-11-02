package handler

import (
	"context"
	"errors"
	"testing"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/service/system"
)

type stubCLI struct{ value valueobject.CLIStatus }

func (s stubCLI) Detect(context.Context) valueobject.CLIStatus { return s.value }

type stubWebView2 struct{ value valueobject.WebView2Status }

func (s stubWebView2) Detect(context.Context) valueobject.WebView2Status { return s.value }

type stubLogs struct {
	limit int
	err   error
}

func (s *stubLogs) Tail(_ context.Context, limit int) ([]valueobject.LogEntry, error) {
	s.limit = limit
	if s.err != nil {
		return nil, s.err
	}
	return []valueobject.LogEntry{{Time: "2026-10-31T10:00:00+08:00", Level: "info", Message: "hello"}}, nil
}

func newTestHandler(logs *stubLogs) *SystemService {
	service := system.New(
		valueobject.Layout{
			Root:        `C:\app`,
			DataDir:     `C:\app\data`,
			LogsDir:     `C:\app\logs`,
			WebView2Dir: `C:\app\webview2`,
			LogFile:     `C:\app\logs\app.log`,
			RootSource:  "exe",
			Portable:    true,
		},
		"0.1.0",
		stubCLI{value: valueobject.CLIStatus{Installed: true, Path: `C:\tools\claude.cmd`, Version: "2.1.224"}},
		stubWebView2{value: valueobject.WebView2Status{Installed: true, Version: "156.0.0"}},
		logs,
	)
	return NewSystemService(service)
}

func TestGetEnvironmentMapsDTO(t *testing.T) {
	h := newTestHandler(&stubLogs{})
	env := h.GetEnvironment()
	if env.Version != "0.1.0" {
		t.Fatalf("version = %q", env.Version)
	}
	if env.Build.WailsVersion != wailsVersion || env.Build.GoVersion == "" {
		t.Fatalf("build = %+v", env.Build)
	}
	if env.CLI.Version != "2.1.224" || !env.CLI.Installed {
		t.Fatalf("cli = %+v", env.CLI)
	}
	if env.Layout.Root != `C:\app` || !env.Layout.Portable {
		t.Fatalf("layout = %+v", env.Layout)
	}
	if env.CheckedAt == "" {
		t.Fatal("checkedAt is empty")
	}
}

func TestGetRecentLogsClampsLimit(t *testing.T) {
	logs := &stubLogs{}
	h := newTestHandler(logs)

	if _, err := h.GetRecentLogs(0); err != nil {
		t.Fatal(err)
	}
	if logs.limit != defaultLogLimit {
		t.Fatalf("limit = %d, want %d", logs.limit, defaultLogLimit)
	}

	if _, err := h.GetRecentLogs(999); err != nil {
		t.Fatal(err)
	}
	if logs.limit != maxLogLimit {
		t.Fatalf("limit = %d, want %d", logs.limit, maxLogLimit)
	}
}

func TestGetRecentLogsMapsError(t *testing.T) {
	h := newTestHandler(&stubLogs{err: errors.New("boom")})
	if _, err := h.GetRecentLogs(10); err == nil {
		t.Fatal("error = nil, want mapped error")
	}
}
