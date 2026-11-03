package persistence

import (
	"context"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

type RunStoreProvider struct {
	layout valueobject.Layout
}

func NewRunStoreProvider(layout valueobject.Layout) *RunStoreProvider {
	return &RunStoreProvider{layout: layout}
}

func (p *RunStoreProvider) ForProject(ctx context.Context, projectID string) (port.RunStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	projectPaths, err := paths.ResolveProject(p.layout, projectID)
	if err != nil {
		return nil, err
	}
	return NewRunStore(projectPaths.Dir), nil
}
