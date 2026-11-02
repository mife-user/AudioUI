package system

import (
	"context"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

type fakeCLI struct {
	calls int
	value valueobject.CLIStatus
}

func (f *fakeCLI) Detect(context.Context) valueobject.CLIStatus {
	f.calls++
	return f.value
}

type fakeWebView2 struct {
	calls int
	value valueobject.WebView2Status
}

func (f *fakeWebView2) Detect(context.Context) valueobject.WebView2Status {
	f.calls++
	return f.value
}

type fakeLogs struct {
	limit int
	value []valueobject.LogEntry
}

func (f *fakeLogs) Tail(_ context.Context, limit int) ([]valueobject.LogEntry, error) {
	f.limit = limit
	return f.value, nil
}

func TestEnvironmentCachesUntilRefresh(t *testing.T) {
	cli := &fakeCLI{value: valueobject.CLIStatus{Installed: true, Version: "2.1.224"}}
	webview := &fakeWebView2{value: valueobject.WebView2Status{Installed: true, Version: "156"}}
	layout := valueobject.Layout{Root: `C:\app`}
	service := New(layout, "0.1.0", cli, webview, &fakeLogs{})

	first := service.Environment(context.Background())
	if first.Version != "0.1.0" || first.CLI.Version != "2.1.224" {
		t.Fatalf("unexpected environment: %+v", first)
	}
	_ = service.Environment(context.Background())
	if cli.calls != 1 || webview.calls != 1 {
		t.Fatalf("probes should be cached, got cli=%d webview=%d", cli.calls, webview.calls)
	}

	_ = service.Refresh(context.Background())
	if cli.calls != 2 || webview.calls != 2 {
		t.Fatalf("refresh should re-probe, got cli=%d webview=%d", cli.calls, webview.calls)
	}
}

func TestRecentLogsDelegates(t *testing.T) {
	logs := &fakeLogs{value: []valueobject.LogEntry{{Message: "hello"}}}
	service := New(valueobject.Layout{}, "0.1.0", &fakeCLI{}, &fakeWebView2{}, logs)

	entries, err := service.RecentLogs(context.Background(), 42)
	if err != nil {
		t.Fatalf("RecentLogs() error = %v", err)
	}
	if logs.limit != 42 || len(entries) != 1 || entries[0].Message != "hello" {
		t.Fatalf("unexpected entries=%+v limit=%d", entries, logs.limit)
	}
}
