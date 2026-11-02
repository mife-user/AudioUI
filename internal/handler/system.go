package handler

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/service/system"
)

const (
	defaultLogLimit = 150
	maxLogLimit     = 200
)

var wailsVersion = detectWailsVersion()

func detectWailsVersion() string {
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, dep := range info.Deps {
			if dep.Path == "github.com/wailsapp/wails/v3" {
				return strings.TrimPrefix(dep.Version, "v")
			}
		}
	}
	return "unknown"
}

// SystemService is the Wails-bound facade for the M0 self-check shell.
// It only converts domain values and maps errors; no business rules live here.
type SystemService struct {
	service *system.Service
}

func NewSystemService(service *system.Service) *SystemService {
	return &SystemService{service: service}
}

type BuildDTO struct {
	WailsVersion string `json:"wailsVersion"`
	GoVersion    string `json:"goVersion"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
}

type LayoutDTO struct {
	Root        string `json:"root"`
	DataDir     string `json:"dataDir"`
	LogsDir     string `json:"logsDir"`
	WebView2Dir string `json:"webview2Dir"`
	LogFile     string `json:"logFile"`
	RootSource  string `json:"rootSource"`
	Portable    bool   `json:"portable"`
}

type CLIStatusDTO struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Error     string `json:"error"`
}

type WebView2StatusDTO struct {
	Installed bool   `json:"installed"`
	Path      string `json:"path"`
	Version   string `json:"version"`
	Error     string `json:"error"`
}

type EnvironmentDTO struct {
	Version   string            `json:"version"`
	Build     BuildDTO          `json:"build"`
	Layout    LayoutDTO         `json:"layout"`
	CLI       CLIStatusDTO      `json:"cli"`
	WebView2  WebView2StatusDTO `json:"webview2"`
	CheckedAt string            `json:"checkedAt"`
}

type LogEntryDTO struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

func (h *SystemService) GetEnvironment() EnvironmentDTO {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return toEnvironmentDTO(h.service.Environment(ctx))
}

func (h *SystemService) RefreshEnvironment() EnvironmentDTO {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return toEnvironmentDTO(h.service.Refresh(ctx))
}

func (h *SystemService) GetRecentLogs(limit int) ([]LogEntryDTO, error) {
	if limit <= 0 {
		limit = defaultLogLimit
	}
	if limit > maxLogLimit {
		limit = maxLogLimit
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	entries, err := h.service.RecentLogs(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("读取日志失败: %w", err)
	}
	result := make([]LogEntryDTO, 0, len(entries))
	for _, entry := range entries {
		result = append(result, LogEntryDTO{
			Time:    entry.Time,
			Level:   entry.Level,
			Message: entry.Message,
		})
	}
	return result, nil
}

func toEnvironmentDTO(info valueobject.EnvironmentInfo) EnvironmentDTO {
	return EnvironmentDTO{
		Version: info.Version,
		Build: BuildDTO{
			WailsVersion: wailsVersion,
			GoVersion:    runtime.Version(),
			OS:           runtime.GOOS,
			Arch:         runtime.GOARCH,
		},
		Layout: LayoutDTO{
			Root:        info.Layout.Root,
			DataDir:     info.Layout.DataDir,
			LogsDir:     info.Layout.LogsDir,
			WebView2Dir: info.Layout.WebView2Dir,
			LogFile:     info.Layout.LogFile,
			RootSource:  info.Layout.RootSource,
			Portable:    info.Layout.Portable,
		},
		CLI: CLIStatusDTO{
			Installed: info.CLI.Installed,
			Path:      info.CLI.Path,
			Version:   info.CLI.Version,
			Error:     info.CLI.Error,
		},
		WebView2: WebView2StatusDTO{
			Installed: info.WebView2.Installed,
			Path:      info.WebView2.Path,
			Version:   info.WebView2.Version,
			Error:     info.WebView2.Error,
		},
		CheckedAt: info.CheckedAt.Format(time.RFC3339),
	}
}
