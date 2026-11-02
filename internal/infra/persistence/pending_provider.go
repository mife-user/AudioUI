package persistence

import (
	"context"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

type PendingStoreProvider struct {
	layout valueobject.Layout
}

func NewPendingStoreProvider(layout valueobject.Layout) *PendingStoreProvider {
	return &PendingStoreProvider{layout: layout}
}

func (p *PendingStoreProvider) ForProject(ctx context.Context, projectID string) (port.PendingStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	projectPaths, err := paths.ResolveProject(p.layout, projectID)
	if err != nil {
		return nil, err
	}
	return NewPendingStore(projectPaths.Dir), nil
}
