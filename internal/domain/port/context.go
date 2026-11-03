package port

import (
	"context"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

type ContextHasher interface {
	InputHashes(ctx context.Context, project entity.Project, nodeID string) (valueobject.ContextHashes, error)
}

// RunContextBuilder constructs the one-shot prompt for an executable node.
type RunContextBuilder interface {
	BuildRunContext(ctx context.Context, project entity.Project, nodeID string, workspace RunWorkspace, budget int) (RunContext, error)
}

type RunContext struct {
	TreeID         string
	NodeID         string
	ChainPath      []string
	OutputKind     valueobject.OutputKind
	ContextVersion int
	InitialPrompt  string
	Hashes         valueobject.ContextHashes
	Estimate       valueobject.ContextEstimate
}
