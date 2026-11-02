package environment

import (
	"context"

	"audio-ui/internal/domain/valueobject"
)

// WebView2Probe checks for the system WebView2 runtime.
type WebView2Probe struct {
	detect func(context.Context) valueobject.WebView2Status
}

func NewWebView2Probe() *WebView2Probe {
	return &WebView2Probe{detect: detectWebView2}
}

func (p *WebView2Probe) Detect(ctx context.Context) valueobject.WebView2Status {
	return p.detect(ctx)
}
