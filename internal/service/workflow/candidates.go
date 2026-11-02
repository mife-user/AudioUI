package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

// CandidateCounter returns how many candidate ports an applied topic exposes.
// Topic output is read from the latest applied round's artifact.
type CandidateCounter interface {
	Count(ctx context.Context, projectID string, node entity.Node) (int, error)
}

type artifactCandidateCounter struct {
	stores port.ArtifactStoreProvider
}

func NewArtifactCandidateCounter(stores port.ArtifactStoreProvider) CandidateCounter {
	return &artifactCandidateCounter{stores: stores}
}

func (c *artifactCandidateCounter) Count(ctx context.Context, projectID string, node entity.Node) (int, error) {
	if node.Type != valueobject.NodeTypeTopic || len(node.Rounds) == 0 {
		return 0, nil
	}
	latest := node.Rounds[0]
	for _, round := range node.Rounds[1:] {
		if round.N > latest.N {
			latest = round
		}
	}
	if latest.Kind != valueobject.OutputKindTopics {
		return 0, nil
	}
	if c.stores == nil {
		return 0, fmt.Errorf("artifact store unavailable")
	}
	store, err := c.stores.ForProject(ctx, projectID)
	if err != nil {
		return 0, err
	}
	artifact, payload, err := store.Get(ctx, latest.ArtifactID)
	if err != nil {
		return 0, err
	}
	if artifact.Kind != valueobject.OutputKindTopics {
		return 0, nil
	}
	var document struct {
		Topics []json.RawMessage `json:"topics"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		return 0, err
	}
	return len(document.Topics), nil
}
