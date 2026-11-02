package environment

import (
	"context"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func TestWebView2ProbeDelegates(t *testing.T) {
	called := false
	probe := &WebView2Probe{
		detect: func(ctx context.Context) valueobject.WebView2Status {
			called = true
			if ctx == nil {
				t.Fatal("context is nil")
			}
			return valueobject.WebView2Status{Installed: true, Version: "test"}
		},
	}
	status := probe.Detect(context.Background())
	if !called {
		t.Fatal("detect was not called")
	}
	if !status.Installed || status.Version != "test" {
		t.Fatalf("status = %+v", status)
	}
}
