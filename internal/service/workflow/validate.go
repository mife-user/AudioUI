package workflow

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

func (s *Service) loadCurrent(ctx context.Context) (entity.Project, *entity.Graph, error) {
	project, err := s.opts.Project.Current(ctx)
	if err != nil {
		return entity.Project{}, nil, err
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		return entity.Project{}, nil, fmt.Errorf("图状态无效: %w", err)
	}
	if err := graph.Validate(); err != nil {
		return entity.Project{}, nil, fmt.Errorf("图状态无效: %w", err)
	}
	return project, graph, nil
}

func requireNode(graph *entity.Graph, id, what string) (*entity.Node, error) {
	node, ok := graph.Node(id)
	if !ok {
		return nil, fmt.Errorf("%w: %s %s", ErrNodeNotFound, what, id)
	}
	return node, nil
}

func chainRoot(graph *entity.Graph, nodeID string) (string, error) {
	path, err := graph.PathToRoot(nodeID)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotInChain, err)
	}
	root, ok := graph.Node(path[0])
	if !ok || root.Type != valueobject.NodeTypeTopic {
		return "", fmt.Errorf("%w: %s", ErrNotInChain, nodeID)
	}
	return root.ID, nil
}

func (s *Service) resolveCandidate(ctx context.Context, projectID string, parent *entity.Node, requested *int) (*int, error) {
	if parent.Type == valueobject.NodeTypePrompt {
		return nil, nil
	}
	if parent.Type != valueobject.NodeTypeTopic {
		return nil, fmt.Errorf("%w: %s", ErrUnknownNodeType, parent.Type)
	}
	if requested == nil || *requested < 0 {
		return nil, fmt.Errorf("%w: topic parent requires a 0-based candidate", ErrCandidateInvalid)
	}
	if s.opts.Candidates == nil {
		return nil, fmt.Errorf("%w: candidate source unavailable", ErrCandidateInvalid)
	}
	count, err := s.opts.Candidates.Count(ctx, projectID, *parent)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCandidateInvalid, err)
	}
	if count <= 0 {
		return nil, fmt.Errorf("%w: topic %s has no applied candidates", ErrCandidateInvalid, parent.ID)
	}
	if *requested >= count {
		return nil, fmt.Errorf("%w: %d not in 0..%d", ErrCandidateInvalid, *requested, count-1)
	}
	value := *requested
	return &value, nil
}

func sameCandidate(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func markSubtreeStale(project *entity.Project, ids []string) {
	index := make(map[string]*entity.Node, len(project.Nodes))
	for i := range project.Nodes {
		index[project.Nodes[i].ID] = &project.Nodes[i]
	}
	for _, id := range ids {
		if node, ok := index[id]; ok {
			node.Stale = true
		}
	}
}

func normalizeTitle(title string, nodeType valueobject.NodeType) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		switch nodeType {
		case valueobject.NodeTypeTopic:
			title = "新选题"
		case valueobject.NodeTypePrompt:
			title = "新提示词"
		}
	}
	if utf8.RuneCountInString(title) > valueobject.MaxNodeTitleLength {
		return "", fmt.Errorf("%w: title is too long", ErrInvalidInput)
	}
	return title, nil
}

func validatePosition(x, y float64) error {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.IsNaN(y) || math.IsInf(y, 0) {
		return fmt.Errorf("%w: coordinates must be finite", ErrInvalidPosition)
	}
	return nil
}

func validateParams(params entity.NodeParams) error {
	if params.OutputKind != "" && !params.OutputKind.Valid() {
		return fmt.Errorf("%w: outputKind %q", ErrInvalidInput, params.OutputKind)
	}
	if params.CandidateCount < 0 {
		return fmt.Errorf("%w: candidateCount must not be negative", ErrInvalidInput)
	}
	return nil
}

func validateFinalGraph(project *entity.Project) error {
	graph, err := entity.NewGraph(project)
	if err != nil {
		return err
	}
	return graph.Validate()
}
