package workspace

import (
	"context"

	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type RunProvider struct {
	layout valueobject.Layout
}

func NewRunProvider(layout valueobject.Layout) *RunProvider {
	return &RunProvider{layout: layout}
}

func (p *RunProvider) OpenRun(ctx context.Context, projectID, treeID, nodeID string, chainPath []string) (port.RunWorkspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return NewRunView(p.layout, projectID, treeID, nodeID, chainPath)
}
