package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/logging"

	"go.uber.org/zap"
)

type selfCheckReport struct {
	Version     string          `json:"version"`
	CheckedAt   string          `json:"checkedAt"`
	Root        string          `json:"root"`
	RootSource  string          `json:"rootSource"`
	Portable    bool            `json:"portable"`
	DataDir     string          `json:"dataDir"`
	LogsDir     string          `json:"logsDir"`
	LogFile     string          `json:"logFile"`
	WebView2Dir string          `json:"webview2Dir"`
	Directories map[string]bool `json:"directories"`
	CLI         cliReport       `json:"cli"`
	WebView2    webView2Report  `json:"webview2"`
	OK          bool            `json:"ok"`
}

type cliReport struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Error     string `json:"error"`
}

type webView2Report struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Error     string `json:"error"`
}

func runSelfCheck(
	ctx context.Context,
	layout valueobject.Layout,
	version string,
	cliProbe port.CLIProbe,
	webviewProbe port.WebView2Probe,
	logger *logging.Logger,
) int {
	cli := cliProbe.Detect(ctx)
	webview := webviewProbe.Detect(ctx)

	directories := map[string]bool{
		"data":     isDir(layout.DataDir),
		"logs":     isDir(layout.LogsDir),
		"webview2": isDir(layout.WebView2Dir),
	}

	report := selfCheckReport{
		Version:     version,
		CheckedAt:   time.Now().Format(time.RFC3339),
		Root:        layout.Root,
		RootSource:  layout.RootSource,
		Portable:    layout.Portable,
		DataDir:     layout.DataDir,
		LogsDir:     layout.LogsDir,
		LogFile:     layout.LogFile,
		WebView2Dir: layout.WebView2Dir,
		Directories: directories,
		CLI: cliReport{
			Installed: cli.Installed,
			Path:      cli.Path,
			Version:   cli.Version,
			Error:     cli.Error,
		},
		WebView2: webView2Report{
			Installed: webview.Installed,
			Path:      webview.Path,
			Version:   webview.Version,
			Error:     webview.Error,
		},
	}
	report.OK = webview.Installed && directories["data"] && directories["logs"] && directories["webview2"]

	payload, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		logger.Zap.Error("self-check marshal failed", zap.Error(err))
		fmt.Fprintf(os.Stderr, "self-check 失败: %v\n", err)
		return 1
	}
	fmt.Println(string(payload))

	reportPath := filepath.Join(layout.LogsDir, "self-check.json")
	if err := os.WriteFile(reportPath, append(payload, '\n'), 0o644); err != nil {
		logger.Zap.Error("self-check report write failed", zap.Error(err))
		fmt.Fprintf(os.Stderr, "无法写入自检报告: %v\n", err)
		return 1
	}

	logger.Zap.Info("self-check completed",
		zap.Bool("ok", report.OK),
		zap.Bool("cli_installed", cli.Installed),
		zap.Bool("webview2_installed", webview.Installed),
		zap.String("report", reportPath),
	)

	if !report.OK {
		return 1
	}
	return 0
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
