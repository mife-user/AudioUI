package workflow

import (
	"context"
	"encoding/json"
	"fmt"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type TopicCandidateReader interface {
	Read(ctx context.Context, projectID string, node entity.Node) ([]entity.TopicCandidate, error)
}

type artifactCandidateCounter struct {
	stores port.ArtifactStoreProvider
}

func NewArtifactCandidateCounter(stores port.ArtifactStoreProvider) *artifactCandidateCounter {
	return &artifactCandidateCounter{stores: stores}
}

func (c *artifactCandidateCounter) Count(ctx context.Context, projectID string, node entity.Node) (int, error) {
	if node.Type != valueobject.NodeTypeTopic || len(node.Rounds) == 0 {
		return 0, nil
	}
	latest := latestRound(node.Rounds)
	if latest.Kind != valueobject.OutputKindTopics {
		return 0, nil
	}
	candidates, err := c.Read(ctx, projectID, node)
	if err != nil {
		return 0, err
	}
	return len(candidates), nil
}

func (c *artifactCandidateCounter) Read(ctx context.Context, projectID string, node entity.Node) ([]entity.TopicCandidate, error) {
	if node.Type != valueobject.NodeTypeTopic {
		return nil, fmt.Errorf("candidate source requires a topic node")
	}
	if len(node.Rounds) == 0 {
		return []entity.TopicCandidate{}, nil
	}
	latest := latestRound(node.Rounds)
	if latest.Kind != valueobject.OutputKindTopics {
		return nil, fmt.Errorf("latest topic round kind is %q", latest.Kind)
	}
	if latest.ArtifactID == "" {
		return nil, fmt.Errorf("latest topic round has no artifact ID")
	}
	if c.stores == nil {
		return nil, fmt.Errorf("artifact store unavailable")
	}
	store, err := c.stores.ForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	artifact, payload, err := store.Get(ctx, latest.ArtifactID)
	if err != nil {
		return nil, err
	}
	if artifact.ID != latest.ArtifactID {
		return nil, fmt.Errorf("artifact ID does not match latest topic round")
	}
	if err := valueobject.ValidateStructuredPayload(latest.Kind, payload); err != nil {
		return nil, fmt.Errorf("artifact payload does not match latest topic round: %w", err)
	}
	var document struct {
		Topics []struct {
			Title string `json:"title"`
			Angle string `json:"angle"`
			Hook  string `json:"hook"`
		} `json:"topics"`
	}
	if err := json.Unmarshal(payload, &document); err != nil {
		return nil, err
	}
	candidates := make([]entity.TopicCandidate, len(document.Topics))
	for index, candidate := range document.Topics {
		candidates[index] = entity.TopicCandidate{
			Index: index, Title: candidate.Title, Angle: candidate.Angle, Hook: candidate.Hook,
		}
	}
	return candidates, nil
}

func (s *Service) GetTopicCandidates(ctx context.Context, nodeID, expectedProjectID string) ([]entity.TopicCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if nodeID == "" || expectedProjectID == "" {
		return nil, fmt.Errorf("%w: node and project IDs are required", ErrInvalidInput)
	}
	var candidates []entity.TopicCandidate
	err := s.opts.Project.WithExclusive(ctx, func(access port.ProjectAccess) error {
		project, err := access.Current(ctx)
		if err != nil {
			return err
		}
		if project.ID != expectedProjectID {
			return fmt.Errorf("%w: expected %s, active %s", ErrProjectChanged, expectedProjectID, project.ID)
		}
		graph, err := entity.NewGraph(&project)
		if err != nil {
			return fmt.Errorf("图状态无效: %w", err)
		}
		if err := graph.ValidateForest(); err != nil {
			return fmt.Errorf("图状态无效: %w", err)
		}
		node, err := requireNode(graph, nodeID, "node")
		if err != nil {
			return err
		}
		if node.Type != valueobject.NodeTypeTopic {
			return fmt.Errorf("%w: node %s is not a topic", ErrUnknownNodeType, nodeID)
		}
		if s.opts.CandidateReader == nil {
			return fmt.Errorf("%w: candidate source unavailable", ErrCandidateInvalid)
		}
		candidates, err = s.opts.CandidateReader.Read(ctx, project.ID, *node)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrCandidateInvalid, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return candidates, nil
}

func latestRound(rounds []entity.Round) entity.Round {
	latest := rounds[0]
	for _, round := range rounds[1:] {
		if round.N > latest.N {
			latest = round
		}
	}
	return latest
}
