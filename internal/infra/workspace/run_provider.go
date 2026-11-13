package workspace

import (
	"context"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type RunProvider struct {
	layout valueobject.Layout
}

func NewRunProvider(layout valueobject.Layout) *RunProvider {
	return &RunProvider{layout: layout}
}

func (p *RunProvider) OpenRun(ctx context.Context, projectID string, snapshot entity.RunExecutionSnapshot) (port.RunWorkspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if snapshot.Version >= 3 {
		return NewSnapshotRunView(p.layout, projectID, snapshot)
	}
	return NewRunView(p.layout, projectID, snapshot.TreeID, snapshot.NodeID, snapshot.ChainPath)
}
