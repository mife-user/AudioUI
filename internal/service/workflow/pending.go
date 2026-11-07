package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

type CreatePendingInput struct {
	RunID        string
	ProjectID    string
	TreeID       string
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

func (s *Service) SubmitRunReply(ctx context.Context, scope port.RunScope, kind valueobject.OutputKind, payload []byte, requestID string) (entity.Pending, error) {
	if err := ctx.Err(); err != nil {
		return entity.Pending{}, err
	}
	if len(payload) > valueobject.MaxPendingPayloadSize {
		return entity.Pending{}, ErrPendingPayloadTooLarge
	}
	if requestID != "" && len(requestID) > valueobject.MaxPendingRequestIDLength {
		return entity.Pending{}, fmt.Errorf("%w: requestId is too long", ErrInvalidInput)
	}
	if kind == valueobject.OutputKindSceneDSL {
		return entity.Pending{}, ErrUnsupportedUntilM3
	}
	if kind != valueobject.OutputKindTopics && kind != valueobject.OutputKindScript {
		return entity.Pending{}, valueobject.ErrOutputKindInvalid
	}
	if err := validateRunReplyScope(scope, kind); err != nil {
		return entity.Pending{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Pending{}, err
	}
	var result entity.Pending
	err := s.opts.Project.WithExclusive(ctx, func(access port.ProjectAccess) error {
		project, graph, err := loadProjectGraph(ctx, access)
		if err != nil {
			return err
		}
		snapshot := scope.ExecutionSnapshot
		if project.ID != scope.ProjectID || project.Version != snapshot.ProjectVersion || snapshot.ProjectRevision > project.Revision {
			return port.ErrChainScopeDenied
		}
		if err := s.ensureRunReplyLock(scope); err != nil {
			return err
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
			if existing.RunID != scope.RunID {
				continue
			}
			if !sameRunReply(existing, scope, kind, requestID, payload) {
				if requestID != "" && existing.RequestID == requestID {
					return ErrRequestIDConflict
				}
				return ErrRunResultConflict
			}
			existingPayload, err := store.GetPayload(ctx, existing.ID)
			if err != nil {
				return err
			}
			if bytes.Equal(existingPayload, payload) {
				result = existing
				return nil
			}
			if requestID != "" && existing.RequestID == requestID {
				return ErrRequestIDConflict
			}
			return ErrRunResultConflict
		}
		if err := validateRunSnapshotGraph(graph, snapshot); err != nil {
			return err
		}
		target, ok := graph.Node(scope.NodeID)
		if !ok {
			return port.ErrChainScopeDenied
		}
		output, err := validateSnapshotOutputBinding(graph, target, snapshot)
		if err != nil {
			return err
		}
		if s.isNodeLockedByOtherRun(scope.RunID, target.ID) || output != nil && s.isNodeLockedByOtherRun(scope.RunID, output.ID) {
			return ErrRunningChainLocked
		}

		pendingID, err := valueobject.NewID(valueobject.PendingIDPrefix)
		if err != nil {
			return err
		}
		binding := snapshot.OutputBinding
		pending := entity.Pending{
			ID: pendingID, RunID: scope.RunID, RequestID: requestID,
			ChainID: scope.ChainID, ChainPath: append([]string(nil), scope.ChainPath...),
			NodeID: scope.NodeID, Kind: kind, ContextHash: scope.ContextHash,
			AncestorHash: scope.AncestorHash, ParamsHash: scope.ParamsHash,
			Payload: pendingPayloadPath(pendingID), Summary: pendingSummary(kind, payload),
			OutputBinding: &binding, OutputSchemaVersion: snapshot.OutputContract.SchemaVersion,
			CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
		if binding.Bound {
			pending.ResultNodeID = binding.NodeID
		}

		if validationErr := valueobject.ValidateStructuredPayload(kind, payload); validationErr != nil {
			if errors.Is(validationErr, valueobject.ErrUnsupportedOutput) || errors.Is(validationErr, valueobject.ErrOutputKindInvalid) {
				return validationErr
			}
			if activePendingCount(pendingItems, scope.TreeID) >= valueobject.MaxPendingPerTree {
				return ErrPendingLimit
			}
			pending.Status = valueobject.PendingInvalid
			pending.Error = validationErr.Error()
			result = pending
			_, err = s.commitPendingMutation(ctx, access, project, pending, payload, nil, nil, ChangeReasonPending)
			return err
		}

		canonical, err := valueobject.CanonicalJSON(payload)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPendingInvalid, err)
		}
		if len(canonical) > valueobject.MaxPendingPayloadSize {
			return ErrPendingPayloadTooLarge
		}
		pending.Status = valueobject.PendingApplied
		pending.ArtifactID = artifactID(canonical)
		artifactNodeID := target.ID
		meta := entity.ArtifactMeta{
			Kind: kind, CreatedAt: pending.CreatedAt,
			SourceRunID: scope.RunID, SourcePendingID: pending.ID,
		}
		var artifactStore port.ArtifactStore
		switch target.Type {
		case valueobject.NodeTypeTopic:
			if target.Locked || len(target.Rounds) != 0 {
				return ErrNodeLocked
			}
			candidates, err := decodeTopicCandidates(payload)
			if err != nil {
				return fmt.Errorf("%w: %v", ErrPendingInvalid, err)
			}
			nodeIDs := make([]string, len(candidates))
			candidateNodes := make([]entity.Node, len(candidates))
			topicID := target.ID
			for index, candidate := range candidates {
				id, err := valueobject.NewID(valueobject.NodeIDPrefix)
				if err != nil {
					return err
				}
				nodeIDs[index] = id
				candidateNodes[index] = entity.Node{
					ID: id, Type: valueobject.NodeTypeCandidate,
					Title:           generatedNodeTitle(candidate.Title, fmt.Sprintf("候选 %d", index+1)),
					Parent:          topicID,
					CandidateSource: &entity.CandidateSource{TopicNodeID: topicID, ArtifactID: pending.ArtifactID, Index: index},
					Position:        entity.Position{X: target.Position.X + 420, Y: target.Position.Y + float64(index)*160},
					Rounds:          []entity.Round{},
				}
			}
			pending.CreatedNodeIDs = nodeIDs
			target.Locked = true
			target.Stale = false
			target.Rounds = append(target.Rounds, entity.Round{
				N: nextRound(target.Rounds), Kind: kind, ArtifactID: pending.ArtifactID,
				PendingID: pending.ID, RunID: scope.RunID,
			})
			project.Nodes = append(project.Nodes, candidateNodes...)
			meta.NodeID = topicID
		case valueobject.NodeTypeAgent:
			if output == nil {
				id, err := valueobject.NewID(valueobject.NodeIDPrefix)
				if err != nil {
					return err
				}
				title, err := decodeScriptTitle(payload)
				if err != nil {
					return fmt.Errorf("%w: %v", ErrPendingInvalid, err)
				}
				project.Nodes = append(project.Nodes, entity.Node{
					ID: id, Type: valueobject.NodeTypeScript, Title: generatedNodeTitle(title, "新脚本"),
					Parent: target.ID, Position: entity.Position{X: target.Position.X + 420, Y: target.Position.Y},
					ScriptVersions: []entity.ScriptVersion{}, Rounds: []entity.Round{},
				})
				output = &project.Nodes[len(project.Nodes)-1]
				pending.CreatedNodeIDs = []string{id}
			} else if len(output.ScriptVersions) > 0 {
				if activePendingCount(pendingItems, scope.TreeID) >= valueobject.MaxPendingPerTree {
					return ErrPendingLimit
				}
				pending.Status = valueobject.PendingPending
				pending.ArtifactID = ""
				result = pending
				_, err = s.commitPendingMutation(ctx, access, project, pending, payload, nil, nil, ChangeReasonPending)
				return err
			}
			pending.ResultNodeID = output.ID
			artifactNodeID = output.ID
			meta.NodeID = artifactNodeID
			meta.SourceRunID = scope.RunID
			output.ScriptVersions = append(output.ScriptVersions, entity.ScriptVersion{
				N: len(output.ScriptVersions) + 1, ArtifactID: pending.ArtifactID, RunID: scope.RunID,
			})
			output.Stale = false
			if binding.Bound {
				descendants, err := graph.Subtree(output.ID)
				if err != nil {
					return err
				}
				markSubtreeStale(&project, descendants[1:])
			}
		default:
			return ErrRunEntryUnavailable
		}
		if err := validateFinalGraph(&project); err != nil {
			return fmt.Errorf("图校验失败: %w", err)
		}
		if s.opts.Artifacts == nil {
			return ErrArtifactStoreUnavailable
		}
		artifactStore, err = s.opts.Artifacts.ForProject(ctx, project.ID)
		if err != nil {
			return err
		}
		artifact := &entityArtifact{payload: canonical, meta: meta}
		result = pending
		_, err = s.commitPendingMutation(ctx, access, project, pending, payload, artifactStore, artifact, ChangeReasonGraph)
		return err
	})
	if err != nil {
		return entity.Pending{}, err
	}
	return result, nil
}

func validateRunReplyScope(scope port.RunScope, kind valueobject.OutputKind) error {
	if scope.RunID == "" || scope.ProjectID == "" || scope.TreeID == "" || scope.NodeID == "" ||
		scope.ChainID != "c_"+scope.NodeID || len(scope.ChainPath) == 0 || scope.ChainPath[0] != scope.TreeID ||
		scope.ChainPath[len(scope.ChainPath)-1] != scope.NodeID || scope.ContextHash == "" ||
		scope.AncestorHash == "" || scope.ParamsHash == "" {
		return port.ErrTurnNotActive
	}
	snapshot := scope.ExecutionSnapshot
	if snapshot.Validate() != nil || snapshot.ProjectVersion != valueobject.ProjectFileVersion ||
		snapshot.ProjectID != scope.ProjectID || snapshot.TreeID != scope.TreeID || snapshot.NodeID != scope.NodeID ||
		!slices.Equal(snapshot.ChainPath, scope.ChainPath) {
		return port.ErrChainScopeDenied
	}
	if snapshot.OutputContract.SchemaVersion != 1 || snapshot.OutputContract.Kind != kind {
		return ErrOutputKindMismatch
	}
	target := snapshot.PathNodes[len(snapshot.PathNodes)-1]
	switch target.Type {
	case valueobject.NodeTypeTopic:
		if kind != valueobject.OutputKindTopics || snapshot.OutputBinding.Bound {
			return ErrOutputKindMismatch
		}
	case valueobject.NodeTypeAgent:
		if kind != valueobject.OutputKindScript {
			return ErrOutputKindMismatch
		}
	default:
		return ErrRunEntryUnavailable
	}
	return nil
}

func (s *Service) ensureRunReplyLock(scope port.RunScope) error {
	lock, ok := s.locks[scope.RunID]
	if !ok || lock.RunID != scope.RunID || lock.ProjectID != scope.ProjectID ||
		lock.TreeID != scope.TreeID || lock.NodeID != scope.NodeID || !slices.Equal(lock.Path, scope.ChainPath) {
		return port.ErrTurnNotActive
	}
	expected := make(map[string]struct{}, len(scope.ChainPath)+1)
	for _, id := range scope.ChainPath {
		expected[id] = struct{}{}
	}
	if scope.ExecutionSnapshot.OutputBinding.Bound {
		expected[scope.ExecutionSnapshot.OutputBinding.NodeID] = struct{}{}
	}
	if len(expected) != len(lock.Members) {
		return port.ErrTurnNotActive
	}
	for id := range expected {
		if _, ok := lock.Members[id]; !ok {
			return port.ErrTurnNotActive
		}
		if s.isNodeLockedByOtherRun(scope.RunID, id) {
			return ErrRunningChainLocked
		}
	}
	return nil
}

func sameRunReply(pending entity.Pending, scope port.RunScope, kind valueobject.OutputKind, requestID string, payload []byte) bool {
	binding := scope.ExecutionSnapshot.OutputBinding
	return pending.RunID == scope.RunID && pending.RequestID == requestID &&
		pending.ChainID == scope.ChainID && slices.Equal(pending.ChainPath, scope.ChainPath) &&
		pending.NodeID == scope.NodeID && pending.Kind == kind &&
		pending.ContextHash == scope.ContextHash && pending.AncestorHash == scope.AncestorHash &&
		pending.ParamsHash == scope.ParamsHash && pending.OutputSchemaVersion == scope.ExecutionSnapshot.OutputContract.SchemaVersion &&
		pending.OutputBinding != nil && *pending.OutputBinding == binding && len(payload) > 0
}

func validateRunSnapshotGraph(graph *entity.Graph, snapshot entity.RunExecutionSnapshot) error {
	path, err := graph.PathToRoot(snapshot.NodeID)
	if err != nil || !slices.Equal(path, snapshot.ChainPath) {
		return port.ErrChainScopeDenied
	}
	for _, frozen := range snapshot.PathNodes {
		live, ok := graph.Node(frozen.ID)
		if !ok || live.Type != frozen.Type || live.Title != frozen.Title || live.Parent != frozen.Parent ||
			live.Locked != frozen.Locked || live.Stale != frozen.Stale ||
			!reflect.DeepEqual(live.ParentCandidate, frozen.ParentCandidate) ||
			!reflect.DeepEqual(live.CandidateSource, frozen.CandidateSource) ||
			!reflect.DeepEqual(live.Params, frozen.Params) || !slices.Equal(live.Rounds, frozen.Rounds) ||
			!slices.Equal(live.ScriptVersions, frozen.ScriptVersions) {
			return port.ErrChainScopeDenied
		}
	}
	return nil
}

func validateSnapshotOutputBinding(graph *entity.Graph, target *entity.Node, snapshot entity.RunExecutionSnapshot) (*entity.Node, error) {
	if target.Type != valueobject.NodeTypeAgent {
		return nil, nil
	}
	binding := snapshot.OutputBinding
	var scriptChildren []*entity.Node
	for _, id := range graph.Children(target.ID) {
		child, ok := graph.Node(id)
		if ok && child.Type == valueobject.NodeTypeScript {
			scriptChildren = append(scriptChildren, child)
		}
	}
	if !binding.Bound {
		if len(scriptChildren) != 0 {
			return nil, ErrOutputBindingConflict
		}
		return nil, nil
	}
	if binding.ParentNodeID != target.ID || len(scriptChildren) != 1 || scriptChildren[0].ID != binding.NodeID {
		return nil, ErrOutputBindingConflict
	}
	output := scriptChildren[0]
	if len(output.ScriptVersions) != binding.BaselineVersion {
		return nil, ErrOutputVersionConflict
	}
	if binding.BaselineVersion == 0 {
		if binding.BaselineArtifactID != "" {
			return nil, ErrOutputVersionConflict
		}
	} else {
		latest := output.ScriptVersions[len(output.ScriptVersions)-1]
		if latest.N != binding.BaselineVersion || latest.ArtifactID != binding.BaselineArtifactID {
			return nil, ErrOutputVersionConflict
		}
	}
	return output, nil
}

func activePendingCount(items []entity.Pending, treeID string) int {
	count := 0
	for _, item := range items {
		if len(item.ChainPath) > 0 && item.ChainPath[0] == treeID &&
			(item.Status == valueobject.PendingPending || item.Status == valueobject.PendingInvalid) {
			count++
		}
	}
	return count
}

type topicReplyCandidate struct {
	Title string `json:"title"`
	Angle string `json:"angle"`
	Hook  string `json:"hook"`
}

func decodeTopicCandidates(payload []byte) ([]topicReplyCandidate, error) {
	var result struct {
		Topics []topicReplyCandidate `json:"topics"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return nil, err
	}
	return result.Topics, nil
}

func decodeScriptTitle(payload []byte) (string, error) {
	var result struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		return "", err
	}
	return result.Title, nil
}

func generatedNodeTitle(title, fallback string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = fallback
	}
	runes := []rune(title)
	if len(runes) > valueobject.MaxNodeTitleLength {
		title = string(runes[:valueobject.MaxNodeTitleLength])
	}
	return title
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
		if input.ProjectID != "" && input.ProjectID != project.ID {
			return port.ErrChainScopeDenied
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
		if !ok || root.Type != valueobject.NodeTypeTopic {
			return fmt.Errorf("%w: pending path does not match node %s", ErrNotInChain, node.ID)
		}
		if input.TreeID != "" && root.ID != input.TreeID {
			return port.ErrChainScopeDenied
		}
		if !slices.Equal(path, input.ChainPath) {
			if input.ProjectID != "" {
				return port.ErrChainScopeDenied
			}
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
		activeForTree := activePendingCount(pendingItems, root.ID)
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
		_, err = s.commitPendingMutation(ctx, access, project, result, input.Payload, nil, nil, ChangeReasonPending)
		return err
	})
	if err != nil {
		return entity.Pending{}, err
	}
	return result, nil
}

func (s *Service) ListPending(ctx context.Context, expectedProjectID string) ([]entity.Pending, error) {
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
		if project.ID != expectedProjectID {
			return fmt.Errorf("%w: expected %s, active %s", ErrProjectChanged, expectedProjectID, project.ID)
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

func (s *Service) GetPending(ctx context.Context, id, expectedProjectID string) (entity.PendingDetail, error) {
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
		if project.ID != expectedProjectID {
			return fmt.Errorf("%w: expected %s, active %s", ErrProjectChanged, expectedProjectID, project.ID)
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

func (s *Service) ApplyPending(ctx context.Context, id string, force bool, expectedProjectID string) (entity.Project, error) {
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
		if project.ID != expectedProjectID {
			return fmt.Errorf("%w: expected %s, active %s", ErrProjectChanged, expectedProjectID, project.ID)
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
		if pending.OutputBinding != nil {
			result, err = s.applyScriptOutputPending(ctx, access, project, graph, pending, payload, force)
			return err
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
		if err := s.validatePendingInputHashes(ctx, project, pending, force); err != nil {
			return err
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
		result, err = s.commitPendingMutation(ctx, access, project, pending, payload, artifactStore, &entityArtifact{payload: canonical, meta: meta}, ChangeReasonPending)
		return err
	})
	if err != nil {
		return entity.Project{}, err
	}
	return result, nil
}

func (s *Service) validatePendingInputHashes(ctx context.Context, project entity.Project, pending entity.Pending, force bool) error {
	if s.opts.ContextHasher == nil {
		return ErrContextHasherUnavailable
	}
	hashes, err := s.opts.ContextHasher.InputHashes(ctx, project, pending.NodeID)
	if err != nil {
		return err
	}
	if !force && pending.ParamsHash != hashes.ParamsHash {
		return ErrParamsChanged
	}
	if !force && pending.AncestorHash != hashes.AncestorHash {
		return ErrContextChanged
	}
	return nil
}

func (s *Service) applyScriptOutputPending(ctx context.Context, access port.ProjectAccess, project entity.Project, graph *entity.Graph, pending entity.Pending, payload []byte, force bool) (entity.Project, error) {
	binding := pending.OutputBinding
	if pending.Kind != valueobject.OutputKindScript || pending.OutputSchemaVersion != 1 || binding == nil || !binding.Bound ||
		binding.ParentNodeID != pending.NodeID || pending.ResultNodeID != binding.NodeID {
		return entity.Project{}, ErrOutputBindingConflict
	}
	agent, err := requireNode(graph, pending.NodeID, "Agent")
	if err != nil || agent.Type != valueobject.NodeTypeAgent {
		return entity.Project{}, ErrOutputBindingConflict
	}
	output, err := requireNode(graph, binding.NodeID, "Script")
	if err != nil || output.Type != valueobject.NodeTypeScript || output.Parent != agent.ID {
		return entity.Project{}, ErrOutputBindingConflict
	}
	if s.isNodeLocked(agent.ID) || s.isNodeLocked(output.ID) {
		return entity.Project{}, ErrRunningChainLocked
	}
	if len(output.ScriptVersions) != binding.BaselineVersion {
		return entity.Project{}, ErrOutputVersionConflict
	}
	if binding.BaselineVersion > 0 {
		latest := output.ScriptVersions[len(output.ScriptVersions)-1]
		if latest.N != binding.BaselineVersion || latest.ArtifactID != binding.BaselineArtifactID {
			return entity.Project{}, ErrOutputVersionConflict
		}
	}
	if err := s.validatePendingInputHashes(ctx, project, pending, force); err != nil {
		return entity.Project{}, err
	}
	canonical, err := valueobject.CanonicalJSON(payload)
	if err != nil {
		return entity.Project{}, fmt.Errorf("%w: %v", ErrPendingInvalid, err)
	}
	if len(canonical) > valueobject.MaxPendingPayloadSize {
		return entity.Project{}, ErrPendingPayloadTooLarge
	}
	if s.opts.Artifacts == nil {
		return entity.Project{}, ErrArtifactStoreUnavailable
	}
	artifactStore, err := s.opts.Artifacts.ForProject(ctx, project.ID)
	if err != nil {
		return entity.Project{}, err
	}
	pending.Status = valueobject.PendingApplied
	pending.ArtifactID = artifactID(canonical)
	pending.Error = ""
	output.ScriptVersions = append(output.ScriptVersions, entity.ScriptVersion{
		N: len(output.ScriptVersions) + 1, ArtifactID: pending.ArtifactID, RunID: pending.RunID,
	})
	output.Stale = false
	descendants, err := graph.Subtree(output.ID)
	if err != nil {
		return entity.Project{}, err
	}
	markSubtreeStale(&project, descendants[1:])
	if err := validateFinalGraph(&project); err != nil {
		return entity.Project{}, fmt.Errorf("图校验失败: %w", err)
	}
	meta := entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: output.ID, CreatedAt: pending.CreatedAt,
		SourceRunID: pending.RunID, SourcePendingID: pending.ID,
	}
	return s.commitPendingMutation(ctx, access, project, pending, payload, artifactStore, &entityArtifact{
		payload: canonical, meta: meta,
	}, ChangeReasonPending)
}

func (s *Service) DiscardPending(ctx context.Context, id, expectedProjectID string) (entity.Project, error) {
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
		if project.ID != expectedProjectID {
			return fmt.Errorf("%w: expected %s, active %s", ErrProjectChanged, expectedProjectID, project.ID)
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
		result, err = s.commitPendingMutation(ctx, access, project, pending, payload, nil, nil, ChangeReasonPending)
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

func (s *Service) commitPendingMutation(ctx context.Context, access port.ProjectAccess, project entity.Project, pending entity.Pending, payload []byte, artifactStore port.ArtifactStore, artifact *entityArtifact, changeReason string) (entity.Project, error) {
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
		s.changedWithReason(saved.ID, saved.Revision, changeReason)
		return entity.Project{}, err
	}
	if err := store.DeleteTransaction(ctx); err != nil {
		s.changedWithReason(saved.ID, saved.Revision, changeReason)
		return entity.Project{}, err
	}
	s.changedWithReason(saved.ID, saved.Revision, changeReason)
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
