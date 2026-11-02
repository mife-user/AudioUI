package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type CreatePendingInput struct {
	RunID        string
	ChainID      string
	ChainPath    []string
	NodeID       string
	Kind         valueobject.OutputKind
	ContextHash  string
	AncestorHash string
	ParamsHash   string
	RequestID    string
	Payload      []byte
}

func ValidateStructuredPayload(kind valueobject.OutputKind, payload []byte) error {
	return valueobject.ValidateStructuredPayload(kind, payload)
}

func (s *Service) CreatePending(ctx context.Context, input CreatePendingInput) (entity.Pending, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Pending{}, err
	}
	if len(input.Payload) > valueobject.MaxPendingPayloadSize {
		return entity.Pending{}, ErrPendingPayloadTooLarge
	}
	if err := valueobject.ValidateID(input.RunID); err != nil {
		return entity.Pending{}, fmt.Errorf("%w: runId: %v", ErrInvalidInput, err)
	}
	if input.RequestID != "" && len(input.RequestID) > valueobject.MaxPendingRequestIDLength {
		return entity.Pending{}, fmt.Errorf("%w: requestId is too long", ErrInvalidInput)
	}
	if input.Kind == valueobject.OutputKindSceneDSL {
		return entity.Pending{}, ErrUnsupportedUntilM3
	}
	if input.ContextHash == "" || input.AncestorHash == "" || input.ParamsHash == "" {
		return entity.Pending{}, fmt.Errorf("%w: context hashes are required", ErrInvalidInput)
	}

	var result entity.Pending
	err := s.opts.Project.WithExclusive(ctx, func(access port.ProjectAccess) error {
		project, graph, err := loadProjectGraph(ctx, access)
		if err != nil {
			return err
		}
		node, err := requireNode(graph, input.NodeID, "node")
		if err != nil {
			return err
		}
		expectedKind, err := nodeOutputKind(node)
		if err != nil {
			return err
		}
		if input.Kind != expectedKind {
			return fmt.Errorf("%w: node %s requires %s, got %s", ErrNodeKindMismatch, node.ID, expectedKind, input.Kind)
		}
		if node.Locked || len(node.Rounds) != 0 {
			return fmt.Errorf("%w: %s", ErrNodeLocked, node.ID)
		}
		path, err := graph.PathToRoot(node.ID)
		if err != nil {
			return err
		}
		root, ok := graph.Node(path[0])
		if !ok || root.Type != valueobject.NodeTypeTopic || !slices.Equal(path, input.ChainPath) {
			return fmt.Errorf("%w: pending path does not match node %s", ErrNotInChain, node.ID)
		}
		if input.ChainID != "c_"+node.ID {
			return fmt.Errorf("%w: chainId must match the target leaf", ErrInvalidInput)
		}
		if err := valueobject.ValidateID(input.ChainID); err != nil {
			return fmt.Errorf("%w: chainId: %v", ErrInvalidInput, err)
		}
		store, err := s.pendingStore(ctx, project.ID)
		if err != nil {
			return err
		}
		pendingItems, err := store.List(ctx)
		if err != nil {
			return err
		}
		for _, existing := range pendingItems {
			if existing.RunID != input.RunID {
				continue
			}
			if input.RequestID != "" && existing.RequestID == input.RequestID {
				existingPayload, err := store.GetPayload(ctx, existing.ID)
				if err != nil {
					return err
				}
				if samePendingRequest(existing, input) && bytes.Equal(existingPayload, input.Payload) {
					result = existing
					return nil
				}
				return ErrRequestIDConflict
			}
			return ErrTurnPendingExists
		}
		activeForTree := 0
		for _, existing := range pendingItems {
			if len(existing.ChainPath) == 0 || existing.ChainPath[0] != root.ID {
				continue
			}
			if existing.Status == valueobject.PendingPending || existing.Status == valueobject.PendingInvalid {
				activeForTree++
			}
		}
		if activeForTree >= valueobject.MaxPendingPerTree {
			return ErrPendingLimit
		}
		id, err := valueobject.NewID(valueobject.PendingIDPrefix)
		if err != nil {
			return err
		}
		status := valueobject.PendingPending
		validationErr := valueobject.ValidateStructuredPayload(input.Kind, input.Payload)
		message := ""
		if validationErr != nil {
			if errors.Is(validationErr, valueobject.ErrUnsupportedOutput) || errors.Is(validationErr, valueobject.ErrOutputKindInvalid) {
				return validationErr
			}
			status = valueobject.PendingInvalid
			message = validationErr.Error()
		}
		result = entity.Pending{
			ID: id, RunID: input.RunID, RequestID: input.RequestID,
			ChainID: input.ChainID, ChainPath: append([]string(nil), input.ChainPath...),
			NodeID: input.NodeID, Kind: input.Kind, Status: status,
			ContextHash: input.ContextHash, AncestorHash: input.AncestorHash,
			ParamsHash: input.ParamsHash, Payload: pendingPayloadPath(id),
			Summary: pendingSummary(input.Kind, input.Payload), Error: message,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		_, err = s.commitPendingMutation(ctx, access, project, result, input.Payload, nil, nil)
		return err
	})
	if err != nil {
		return entity.Pending{}, err
	}
	return result, nil
}

func (s *Service) ListPending(ctx context.Context) ([]entity.Pending, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var result []entity.Pending
	err := s.opts.Project.WithExclusive(ctx, func(access port.ProjectAccess) error {
		project, err := access.Current(ctx)
		if err != nil {
			return err
		}
		store, err := s.pendingStore(ctx, project.ID)
		if err != nil {
			return err
		}
		result, err = store.List(ctx)
		return err
	})
	return result, err
}

func (s *Service) GetPending(ctx context.Context, id string) (entity.PendingDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.PendingDetail{}, err
	}
	if err := valueobject.ValidateID(id); err != nil {
		return entity.PendingDetail{}, fmt.Errorf("%w: %v", ErrPendingNotFound, err)
	}
	var result entity.PendingDetail
	err := s.opts.Project.WithExclusive(ctx, func(access port.ProjectAccess) error {
		project, err := access.Current(ctx)
		if err != nil {
			return err
		}
		store, err := s.pendingStore(ctx, project.ID)
		if err != nil {
			return err
		}
		pending, err := store.Load(ctx, id)
		if err != nil {
			return err
		}
		payload, err := store.GetPayload(ctx, id)
		if err != nil {
			return err
		}
		result = entity.PendingDetail{Pending: pending, RawPayload: payload}
		return nil
	})
	return result, err
}

func (s *Service) ApplyPending(ctx context.Context, id string, force bool) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	var result entity.Project
	err := s.opts.Project.WithExclusive(ctx, func(access port.ProjectAccess) error {
		project, graph, err := loadProjectGraph(ctx, access)
		if err != nil {
			return err
		}
		store, err := s.pendingStore(ctx, project.ID)
		if err != nil {
			return err
		}
		pending, err := store.Load(ctx, id)
		if err != nil {
			return err
		}
		switch pending.Status {
		case valueobject.PendingApplied:
			return ErrPendingAlreadyApplied
		case valueobject.PendingDiscarded:
			return ErrPendingNotApplicable
		case valueobject.PendingInvalid:
			return ErrPendingInvalid
		case valueobject.PendingPending:
		default:
			return ErrPendingNotApplicable
		}
		payload, err := store.GetPayload(ctx, id)
		if err != nil {
			return err
		}
		if err := valueobject.ValidateStructuredPayload(pending.Kind, payload); err != nil {
			return fmt.Errorf("%w: %v", ErrPendingInvalid, err)
		}
		node, err := requireNode(graph, pending.NodeID, "node")
		if err != nil {
			return err
		}
		expectedKind, err := nodeOutputKind(node)
		if err != nil {
			return err
		}
		if expectedKind != pending.Kind {
			return fmt.Errorf("%w: node %s requires %s, pending has %s", ErrNodeKindMismatch, node.ID, expectedKind, pending.Kind)
		}
		if s.isNodeLocked(node.ID) {
			return fmt.Errorf("%w: %s", ErrRunningChainLocked, node.ID)
		}
		if node.Locked || len(node.Rounds) != 0 {
			return fmt.Errorf("%w: %s", ErrNodeLocked, node.ID)
		}
		if s.opts.ContextHasher == nil {
			return ErrContextHasherUnavailable
		}
		hashes, err := s.opts.ContextHasher.InputHashes(ctx, project, node.ID)
		if err != nil {
			return err
		}
		if !force && pending.ParamsHash != hashes.ParamsHash {
			return ErrParamsChanged
		}
		if !force && pending.AncestorHash != hashes.AncestorHash {
			return ErrContextChanged
		}
		canonical, err := valueobject.CanonicalJSON(payload)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPendingInvalid, err)
		}
		if len(canonical) > valueobject.MaxPendingPayloadSize {
			return ErrPendingPayloadTooLarge
		}
		if s.opts.Artifacts == nil {
			return ErrArtifactStoreUnavailable
		}
		artifactStore, err := s.opts.Artifacts.ForProject(ctx, project.ID)
		if err != nil {
			return err
		}
		artifactID := artifactID(canonical)
		meta := entity.ArtifactMeta{
			Kind: pending.Kind, NodeID: pending.NodeID, CreatedAt: pending.CreatedAt,
			SourceRunID: pending.RunID, SourcePendingID: pending.ID,
		}
		pending.Status = valueobject.PendingApplied
		pending.ArtifactID = artifactID
		pending.Error = ""
		descendants, err := graph.Subtree(node.ID)
		if err != nil {
			return err
		}
		for i := range project.Nodes {
			if project.Nodes[i].ID == node.ID {
				project.Nodes[i].Locked = true
				project.Nodes[i].Stale = false
				project.Nodes[i].Rounds = append(project.Nodes[i].Rounds, entity.Round{
					N: nextRound(project.Nodes[i].Rounds), Kind: pending.Kind,
					ArtifactID: artifactID, PendingID: pending.ID, RunID: pending.RunID,
				})
			}
		}
		markSubtreeStale(&project, descendants[1:])
		result, err = s.commitPendingMutation(ctx, access, project, pending, payload, artifactStore, &entityArtifact{payload: canonical, meta: meta})
		return err
	})
	if err != nil {
		return entity.Project{}, err
	}
	return result, nil
}

func (s *Service) DiscardPending(ctx context.Context, id string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	var result entity.Project
	err := s.opts.Project.WithExclusive(ctx, func(access port.ProjectAccess) error {
		project, err := access.Current(ctx)
		if err != nil {
			return err
		}
		store, err := s.pendingStore(ctx, project.ID)
		if err != nil {
			return err
		}
		pending, err := store.Load(ctx, id)
		if err != nil {
			return err
		}
		if pending.Status == valueobject.PendingApplied {
			return ErrPendingAlreadyApplied
		}
		if pending.Status == valueobject.PendingDiscarded {
			result = project
			return nil
		}
		payload, err := store.GetPayload(ctx, id)
		if err != nil {
			return err
		}
		pending.Status = valueobject.PendingDiscarded
		result, err = s.commitPendingMutation(ctx, access, project, pending, payload, nil, nil)
		return err
	})
	if err != nil {
		return entity.Project{}, err
	}
	return result, nil
}

type entityArtifact struct {
	payload []byte
	meta    entity.ArtifactMeta
}

func (s *Service) commitPendingMutation(ctx context.Context, access port.ProjectAccess, project entity.Project, pending entity.Pending, payload []byte, artifactStore port.ArtifactStore, artifact *entityArtifact) (entity.Project, error) {
	store, err := s.pendingStore(ctx, project.ID)
	if err != nil {
		return entity.Project{}, err
	}
	baseRevision := project.Revision
	project.Revision++
	transaction := entity.PendingTransaction{
		Version:      valueobject.PendingTransactionFileVersion,
		BaseRevision: baseRevision, TargetRevision: project.Revision,
		Pending: pending, Payload: append([]byte(nil), payload...),
	}
	if artifact != nil {
		transaction.ArtifactPayload = append([]byte(nil), artifact.payload...)
		meta := artifact.meta
		transaction.ArtifactMeta = &meta
	}
	if err := store.SaveTransaction(ctx, transaction); err != nil {
		return entity.Project{}, err
	}
	if artifact != nil {
		if artifactStore == nil {
			if s.opts.Artifacts == nil {
				return entity.Project{}, ErrArtifactStoreUnavailable
			}
			artifactStore, err = s.opts.Artifacts.ForProject(ctx, project.ID)
			if err != nil {
				return entity.Project{}, err
			}
		}
		written, err := artifactStore.Put(ctx, artifact.payload, artifact.meta)
		if err != nil {
			return entity.Project{}, err
		}
		if written.ID != pending.ArtifactID {
			return entity.Project{}, fmt.Errorf("%w: artifact id mismatch", ErrInvalidInput)
		}
	}
	if err := store.WritePayload(ctx, pending.ID, payload); err != nil {
		return entity.Project{}, err
	}
	saved, err := access.Save(ctx, project)
	if err != nil {
		return entity.Project{}, err
	}
	if err := store.Save(ctx, pending, payload); err != nil {
		return entity.Project{}, err
	}
	if err := store.DeleteTransaction(ctx); err != nil {
		s.changedPending(saved.Revision)
		return entity.Project{}, err
	}
	s.changedPending(saved.Revision)
	return saved, nil
}

func (s *Service) pendingStore(ctx context.Context, projectID string) (port.PendingStore, error) {
	if s.opts.Pending == nil {
		return nil, ErrPendingStoreUnavailable
	}
	store, err := s.opts.Pending.ForProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	if store == nil {
		return nil, ErrPendingStoreUnavailable
	}
	return store, nil
}

func loadProjectGraph(ctx context.Context, access port.ProjectAccess) (entity.Project, *entity.Graph, error) {
	project, err := access.Current(ctx)
	if err != nil {
		return entity.Project{}, nil, err
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		return entity.Project{}, nil, err
	}
	if err := graph.Validate(); err != nil {
		return entity.Project{}, nil, err
	}
	return project, graph, nil
}

func nodeOutputKind(node *entity.Node) (valueobject.OutputKind, error) {
	if node.Type == valueobject.NodeTypeTopic {
		return valueobject.OutputKindTopics, nil
	}
	if node.Type != valueobject.NodeTypePrompt {
		return "", ErrUnknownNodeType
	}
	switch node.Params.OutputKind {
	case "", valueobject.OutputKindScript:
		return valueobject.OutputKindScript, nil
	case valueobject.OutputKindSceneDSL:
		return "", ErrUnsupportedUntilM3
	default:
		return "", ErrNodeKindMismatch
	}
}

func samePendingRequest(pending entity.Pending, input CreatePendingInput) bool {
	return pending.RequestID == input.RequestID && pending.ChainID == input.ChainID &&
		pending.NodeID == input.NodeID && pending.Kind == input.Kind &&
		pending.ContextHash == input.ContextHash && pending.AncestorHash == input.AncestorHash &&
		pending.ParamsHash == input.ParamsHash && slices.Equal(pending.ChainPath, input.ChainPath)
}

func pendingSummary(kind valueobject.OutputKind, payload []byte) string {
	var summary struct {
		Title  string `json:"title"`
		Topics []struct {
			Title string `json:"title"`
		} `json:"topics"`
	}
	if json.Unmarshal(payload, &summary) != nil {
		return ""
	}
	if kind == valueobject.OutputKindTopics {
		if len(summary.Topics) > 0 {
			return summary.Topics[0].Title
		}
		return ""
	}
	return summary.Title
}

func pendingPayloadPath(id string) string {
	return "pending/" + id + ".json"
}

func nextRound(rounds []entity.Round) int {
	next := 1
	for _, round := range rounds {
		if round.N >= next {
			next = round.N + 1
		}
	}
	return next
}

func artifactID(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
