//go:build windows

package environment

import (
	"context"
	"testing"
	"time"

	"audio-ui/internal/infra/agent/claudecli"
)

func TestClaudeProbeHonorsCallerDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	probe := &ClaudeProbe{
		resolveNative: func(ctx context.Context, _ string) (claudecli.NativeCLI, error) {
			<-ctx.Done()
			return claudecli.NativeCLI{}, ctx.Err()
		},
	}
	started := time.Now()
	status := probe.Detect(ctx)
	if time.Since(started) > 2*time.Second {
		t.Fatal("CLI probe exceeded caller deadline")
	}
	if status.Installed || status.Error == "" {
		t.Fatalf("status = %+v", status)
	}
}
