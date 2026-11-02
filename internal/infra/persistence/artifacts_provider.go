package persistence

import (
	"context"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

// ArtifactStoreProvider returns per-project artifact stores for workflow
// commands.
type ArtifactStoreProvider struct {
	layout valueobject.Layout
}

func NewArtifactStoreProvider(layout valueobject.Layout) *ArtifactStoreProvider {
	return &ArtifactStoreProvider{layout: layout}
}

func (p *ArtifactStoreProvider) ForProject(ctx context.Context, projectID string) (port.ArtifactStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	projectPaths, err := paths.ResolveProject(p.layout, projectID)
	if err != nil {
		return nil, err
	}
	return NewArtifactStore(projectPaths.Dir), nil
}
