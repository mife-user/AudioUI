package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/logging"
	"audio-ui/internal/infra/paths"
)

type selfCheckCLI struct{ value valueobject.CLIStatus }

func (s selfCheckCLI) Detect(context.Context) valueobject.CLIStatus { return s.value }

type selfCheckWebView2 struct{ value valueobject.WebView2Status }

func (s selfCheckWebView2) Detect(context.Context) valueobject.WebView2Status { return s.value }

func TestRunSelfCheckWritesReport(t *testing.T) {
	root := t.TempDir()
	layout := valueobject.Layout{
		Root:        root,
		DataDir:     filepath.Join(root, "data"),
		LogsDir:     filepath.Join(root, "logs"),
		WebView2Dir: filepath.Join(root, "webview2"),
		LogFile:     filepath.Join(root, "logs", "app.log"),
		RootSource:  "exe",
		Portable:    true,
	}
	if err := paths.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	logger, err := logging.New(layout.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	exitCode := runSelfCheck(
		context.Background(),
		layout,
		"0.1.0",
		selfCheckCLI{value: valueobject.CLIStatus{Installed: false, Error: "missing"}},
		selfCheckWebView2{value: valueobject.WebView2Status{Installed: true, Version: "156"}},
		logger,
	)
	if exitCode != 0 {
		t.Fatalf("exitCode = %d, want 0 when only CLI is missing", exitCode)
	}

	reportBytes, err := os.ReadFile(filepath.Join(layout.LogsDir, "self-check.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		OK  bool `json:"ok"`
		CLI struct {
			Installed bool `json:"installed"`
		} `json:"cli"`
		WebView2 struct {
			Installed bool `json:"installed"`
		} `json:"webview2"`
	}
	if err := json.Unmarshal(reportBytes, &report); err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.CLI.Installed || !report.WebView2.Installed {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestRunSelfCheckFailsWithoutWebView2(t *testing.T) {
	root := t.TempDir()
	layout := valueobject.Layout{
		Root:        root,
		DataDir:     filepath.Join(root, "data"),
		LogsDir:     filepath.Join(root, "logs"),
		WebView2Dir: filepath.Join(root, "webview2"),
		LogFile:     filepath.Join(root, "logs", "app.log"),
		RootSource:  "exe",
		Portable:    true,
	}
	if err := paths.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	logger, err := logging.New(layout.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()

	exitCode := runSelfCheck(
		context.Background(),
		layout,
		"0.1.0",
		selfCheckCLI{value: valueobject.CLIStatus{Installed: true}},
		selfCheckWebView2{value: valueobject.WebView2Status{Installed: false, Error: "missing"}},
		logger,
	)
	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1 without WebView2", exitCode)
	}
}
