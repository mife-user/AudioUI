package system

import (
	"context"
	"sync"
	"time"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

// Service aggregates the M0 environment probes and the recent-log view.
type Service struct {
	layout  valueobject.Layout
	version string
	cli     port.CLIProbe
	webview port.WebView2Probe
	logs    port.LogTailer

	mu     sync.Mutex
	cached *valueobject.EnvironmentInfo
}

func New(
	layout valueobject.Layout,
	version string,
	cli port.CLIProbe,
	webview port.WebView2Probe,
	logs port.LogTailer,
) *Service {
	return &Service{
		layout:  layout,
		version: version,
		cli:     cli,
		webview: webview,
		logs:    logs,
	}
}

func (s *Service) Environment(ctx context.Context) valueobject.EnvironmentInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cached != nil {
		return *s.cached
	}
	return s.probeLocked(ctx)
}

func (s *Service) Refresh(ctx context.Context) valueobject.EnvironmentInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.probeLocked(ctx)
}

func (s *Service) probeLocked(ctx context.Context) valueobject.EnvironmentInfo {
	info := valueobject.EnvironmentInfo{
		Version:   s.version,
		Layout:    s.layout,
		CLI:       s.cli.Detect(ctx),
		WebView2:  s.webview.Detect(ctx),
		CheckedAt: time.Now(),
	}
	s.cached = &info
	return info
}

func (s *Service) RecentLogs(ctx context.Context, limit int) ([]valueobject.LogEntry, error) {
	return s.logs.Tail(ctx, limit)
}
