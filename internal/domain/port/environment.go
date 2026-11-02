package port

import (
	"context"

	"audio-ui/internal/domain/valueobject"
)

// CLIProbe detects the Claude Code CLI without blocking application startup.
type CLIProbe interface {
	Detect(ctx context.Context) valueobject.CLIStatus
}

// WebView2Probe detects the WebView2 runtime before Wails starts.
type WebView2Probe interface {
	Detect(ctx context.Context) valueobject.WebView2Status
}

// LogTailer returns the most recent application log entries, newest first.
type LogTailer interface {
	Tail(ctx context.Context, limit int) ([]valueobject.LogEntry, error)
}
