package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

func (s *Service) ReadRunGraph(ctx context.Context, scope port.RunScope) (port.RunGraph, error) {
	_, graph, err := s.runGraph(ctx, scope)
	if err != nil {
		return port.RunGraph{}, err
	}
	result := port.RunGraph{Nodes: make([]port.RunGraphNode, 0, len(scope.ChainPath))}
	for _, id := range scope.ChainPath {
		node, ok := graph.Node(id)
		if !ok {
			return port.RunGraph{}, port.ErrChainScopeDenied
		}
		result.Nodes = append(result.Nodes, runGraphNode(node))
	}
	return result, nil
}

func (s *Service) ReadRunNode(ctx context.Context, scope port.RunScope, nodeID string) (port.RunNode, error) {
	_, graph, err := s.runGraph(ctx, scope)
	if err != nil {
		return port.RunNode{}, err
	}
	if !slices.Contains(scope.ChainPath, nodeID) {
		return port.RunNode{}, port.ErrChainScopeDenied
	}
	node, ok := graph.Node(nodeID)
	if !ok {
		return port.RunNode{}, port.ErrChainScopeDenied
	}
	rounds := make([]port.RunRound, 0, len(node.Rounds))
	for _, round := range node.Rounds {
		rounds = append(rounds, port.RunRound{
			N: round.N, Kind: round.Kind, ArtifactID: round.ArtifactID,
			PendingID: round.PendingID, RunID: round.RunID,
		})
	}
	return port.RunNode{
		RunGraphNode: runGraphNode(node),
		Params:       node.Params,
		Rounds:       rounds,
	}, nil
}

func (s *Service) ReadRunArtifact(ctx context.Context, scope port.RunScope, artifactID string) (port.RunArtifact, error) {
	project, graph, err := s.runGraph(ctx, scope)
	if err != nil {
		return port.RunArtifact{}, err
	}
	type artifactRef struct {
		kind      valueobject.OutputKind
		candidate int
	}
	refs := make([]artifactRef, 0, 1)
	for i, nodeID := range scope.ChainPath[:len(scope.ChainPath)-1] {
		node, ok := graph.Node(nodeID)
		if !ok {
			return port.RunArtifact{}, port.ErrChainScopeDenied
		}
		for _, round := range node.Rounds {
			if round.ArtifactID != artifactID {
				continue
			}
			ref := artifactRef{kind: round.Kind, candidate: -1}
			if round.Kind == valueobject.OutputKindTopics {
				child, ok := graph.Node(scope.ChainPath[i+1])
				if !ok || child.Parent != node.ID || child.ParentCandidate == nil || *child.ParentCandidate < 0 {
					return port.RunArtifact{}, ErrCandidateInvalid
				}
				ref.candidate = *child.ParentCandidate
			}
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		return port.RunArtifact{}, port.ErrChainScopeDenied
	}
	if s.opts.Artifacts == nil {
		return port.RunArtifact{}, ErrArtifactStoreUnavailable
	}
	store, err := s.opts.Artifacts.ForProject(ctx, project.ID)
	if err != nil {
		return port.RunArtifact{}, err
	}
	artifact, payload, err := store.Get(ctx, artifactID)
	if err != nil {
		if errors.Is(err, port.ErrArtifactNotFound) {
			return port.RunArtifact{}, port.ErrArtifactNotFound
		}
		if errors.Is(err, port.ErrArtifactCorrupt) {
			return port.RunArtifact{}, port.ErrArtifactCorrupt
		}
		return port.RunArtifact{}, fmt.Errorf("%w: %v", port.ErrArtifactCorrupt, err)
	}
	if artifact.ID != artifactID || len(payload) > valueobject.MaxPendingPayloadSize || !utf8.Valid(payload) {
		return port.RunArtifact{}, port.ErrArtifactCorrupt
	}

	var result port.RunArtifact
	for i, ref := range refs {
		if ref.kind == valueobject.OutputKindSceneDSL {
			return port.RunArtifact{}, ErrUnsupportedUntilM3
		}
		if err := valueobject.ValidateStructuredPayload(ref.kind, payload); err != nil {
			return port.RunArtifact{}, fmt.Errorf("%w: %v", port.ErrArtifactCorrupt, err)
		}
		view, err := artifactView(ref.kind, ref.candidate, payload)
		if err != nil {
			return port.RunArtifact{}, err
		}
		if i == 0 {
			result = view
			continue
		}
		if result.Kind != view.Kind || string(result.Data) != string(view.Data) {
			return port.RunArtifact{}, port.ErrChainScopeDenied
		}
	}
	return result, nil
}

func (s *Service) CreateRunPending(ctx context.Context, scope port.RunScope, kind valueobject.OutputKind, payload []byte, requestID string) (entity.Pending, error) {
	if scope.RunID == "" || scope.ProjectID == "" || scope.TreeID == "" || scope.NodeID == "" || scope.ChainID != "c_"+scope.NodeID || len(scope.ChainPath) == 0 || scope.ChainPath[len(scope.ChainPath)-1] != scope.NodeID {
		return entity.Pending{}, port.ErrTurnNotActive
	}
	return s.CreatePending(ctx, CreatePendingInput{
		RunID: scope.RunID, ProjectID: scope.ProjectID, TreeID: scope.TreeID,
		ChainID: scope.ChainID, ChainPath: scope.ChainPath, NodeID: scope.NodeID,
		Kind: kind, ContextHash: scope.ContextHash, AncestorHash: scope.AncestorHash,
		ParamsHash: scope.ParamsHash, RequestID: requestID, Payload: payload,
	})
}

func (s *Service) runGraph(ctx context.Context, scope port.RunScope) (entity.Project, *entity.Graph, error) {
	if err := ctx.Err(); err != nil {
		return entity.Project{}, nil, err
	}
	if scope.RunID == "" || scope.ProjectID == "" || scope.TreeID == "" || scope.NodeID == "" || scope.ChainID != "c_"+scope.NodeID || len(scope.ChainPath) == 0 || scope.ChainPath[len(scope.ChainPath)-1] != scope.NodeID {
		return entity.Project{}, nil, port.ErrTurnNotActive
	}
	project, err := s.opts.Project.Current(ctx)
	if err != nil {
		return entity.Project{}, nil, err
	}
	if project.ID != scope.ProjectID {
		return entity.Project{}, nil, port.ErrChainScopeDenied
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		return entity.Project{}, nil, err
	}
	if err := graph.Validate(); err != nil {
		return entity.Project{}, nil, err
	}
	path, err := graph.PathToRoot(scope.NodeID)
	if err != nil {
		return entity.Project{}, nil, port.ErrChainScopeDenied
	}
	root, ok := graph.Node(path[0])
	if !ok || root.Type != valueobject.NodeTypeTopic || root.ID != scope.TreeID || !slices.Equal(path, scope.ChainPath) {
		return entity.Project{}, nil, port.ErrChainScopeDenied
	}
	return project, graph, nil
}

func runGraphNode(node *entity.Node) port.RunGraphNode {
	var candidate *int
	if node.ParentCandidate != nil {
		value := *node.ParentCandidate
		candidate = &value
	}
	return port.RunGraphNode{
		ID: node.ID, Type: node.Type, Title: node.Title, Parent: node.Parent,
		ParentCandidate: candidate, Locked: node.Locked, Stale: node.Stale,
	}
}

func artifactView(kind valueobject.OutputKind, candidate int, payload []byte) (port.RunArtifact, error) {
	if kind != valueobject.OutputKindTopics {
		data, err := valueobject.CanonicalJSON(payload)
		if err != nil {
			return port.RunArtifact{}, fmt.Errorf("%w: %v", port.ErrArtifactCorrupt, err)
		}
		return port.RunArtifact{Kind: kind, Data: data}, nil
	}
	var document struct {
		Topics []json.RawMessage `json:"topics"`
	}
	if err := json.Unmarshal(payload, &document); err != nil || candidate < 0 || candidate >= len(document.Topics) {
		return port.RunArtifact{}, ErrCandidateInvalid
	}
	data, err := valueobject.CanonicalJSON(document.Topics[candidate])
	if err != nil {
		return port.RunArtifact{}, fmt.Errorf("%w: %v", port.ErrArtifactCorrupt, err)
	}
	return port.RunArtifact{Kind: kind, Data: data}, nil
}
