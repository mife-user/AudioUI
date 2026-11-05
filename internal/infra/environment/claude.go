package environment

import (
	"context"
	"os"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/agent/claudecli"
)

type ClaudeProbe struct {
	resolveNative func(context.Context, string) (claudecli.NativeCLI, error)
}

func NewClaudeProbe() *ClaudeProbe {
	return &ClaudeProbe{resolveNative: claudecli.ResolveNative}
}

func (p *ClaudeProbe) Detect(ctx context.Context) valueobject.CLIStatus {
	native, err := p.resolveNative(ctx, os.Getenv("AUDIOUI_CLAUDE_PATH"))
	if err != nil {
		return valueobject.CLIStatus{Error: err.Error()}
	}
	return valueobject.CLIStatus{Installed: true, Path: native.Path, Version: native.Version}
}
