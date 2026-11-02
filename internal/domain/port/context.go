package port

import (
	"context"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

type ContextHasher interface {
	InputHashes(ctx context.Context, project entity.Project, nodeID string) (valueobject.ContextHashes, error)
}
