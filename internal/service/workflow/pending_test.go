package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/persistence"
)

type pendingStoreProvider struct {
	store port.PendingStore
}

func (p pendingStoreProvider) ForProject(ctx context.Context, projectID string) (port.PendingStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.store, nil
}

type fixedContextHasher struct {
	hashes valueobject.ContextHashes
	err    error
}

func (h *fixedContextHasher) InputHashes(ctx context.Context, _ entity.Project, _ string) (valueobject.ContextHashes, error) {
	if err := ctx.Err(); err != nil {
		return valueobject.ContextHashes{}, err
	}
	return h.hashes, h.err
}

type pendingHarness struct {
	service   *Service
	session   *fakeSession
	pending   *persistence.PendingStore
	artifacts *persistence.ArtifactStore
	hasher    *fixedContextHasher
	changes   *[]changeCall
}

func newPendingHarness(t *testing.T, project entity.Project) *pendingHarness {
	t.Helper()
	session := &fakeSession{project: project}
	pendingStore := persistence.NewPendingStore(t.TempDir())
	artifactStore := persistence.NewArtifactStore(t.TempDir())
	hasher := &fixedContextHasher{hashes: valueobject.ContextHashes{
		ContextHash: "context-v1", AncestorHash: "ancestor-v1", ParamsHash: "params-v1",
	}}
	changes := &[]changeCall{}
	service := New(Options{
		Project:       session,
		Pending:       pendingStoreProvider{store: pendingStore},
		Artifacts:     &fakeArtifactProvider{store: artifactStore},
		ContextHasher: hasher,
		OnChange: func(projectID string, revision int64, reason string) {
			*changes = append(*changes, changeCall{projectID: projectID, revision: revision, reason: reason})
		},
	})
	return &pendingHarness{service: service, session: session, pending: pendingStore, artifacts: artifactStore, hasher: hasher, changes: changes}
}

func validScriptPayload() []byte {
	return []byte(`{"title":"脚本","scenes":[{"narration":"旁白","duration":0}]}`)
}

func pendingInput(runID, nodeID string, path []string, kind valueobject.OutputKind, payload []byte) CreatePendingInput {
	return CreatePendingInput{
		RunID: runID, ChainID: "c_" + nodeID, ChainPath: append([]string(nil), path...),
		NodeID: nodeID, Kind: kind, ContextHash: "context-v1", AncestorHash: "ancestor-v1",
		ParamsHash: "params-v1", Payload: append([]byte(nil), payload...),
	}
}

func TestCreatePendingPreservesInvalidOutputAndRequestIdentity(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	input := pendingInput("r_first", "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, validScriptPayload())
	input.RequestID = "req_first"
	first, err := h.service.CreatePending(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != valueobject.PendingPending || first.RequestID != input.RequestID || h.session.project.Revision != 2 {
		t.Fatalf("pending=%+v revision=%d", first, h.session.project.Revision)
	}
	beforeChanges := len(*h.changes)
	repeated, err := h.service.CreatePending(context.Background(), input)
	if err != nil || repeated.ID != first.ID || h.session.project.Revision != 2 || len(*h.changes) != beforeChanges {
		t.Fatalf("idempotent reply=%+v err=%v revision=%d", repeated, err, h.session.project.Revision)
	}
	changed := input
	changed.Payload = []byte(`{"title":"别的脚本","scenes":[{"narration":"旁白","duration":0}]}`)
	if _, err := h.service.CreatePending(context.Background(), changed); !errors.Is(err, ErrRequestIDConflict) {
		t.Fatalf("same request id with different input error = %v, want REQUEST_ID_CONFLICT", err)
	}
	otherRequest := input
	otherRequest.RequestID = "req_second"
	if _, err := h.service.CreatePending(context.Background(), otherRequest); !errors.Is(err, ErrTurnPendingExists) {
		t.Fatalf("second structured result error = %v, want TURN_PENDING_EXISTS", err)
	}

	invalidPayload := []byte(`{"title":"不完整","scenes":[{"narration":"旁白","duration":"未知"}]}`)
	invalidInput := pendingInput("r_invalid", "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, invalidPayload)
	invalid, err := h.service.CreatePending(context.Background(), invalidInput)
	if err != nil {
		t.Fatal(err)
	}
	if invalid.Status != valueobject.PendingInvalid || invalid.Error == "" {
		t.Fatalf("invalid output was not retained as invalid: %+v", invalid)
	}
	detail, err := h.service.GetPending(context.Background(), invalid.ID)
	if err != nil || !bytes.Equal(detail.RawPayload, invalidPayload) {
		t.Fatalf("raw payload changed: %q err=%v", detail.RawPayload, err)
	}
	if len(*h.changes) != 2 || (*h.changes)[1].projectID != h.session.project.ID || (*h.changes)[1].reason != ChangeReasonPending {
		t.Fatalf("pending change events = %+v", *h.changes)
	}
}

func TestWorkspaceViewContainsDerivedChainsPendingAndProjectRunLocks(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	input := pendingInput("r_workspace", "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, validScriptPayload())
	pending, err := h.service.CreatePending(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	view, err := h.service.WorkspaceView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if view.Project.ID != h.session.project.ID || view.Project.Revision != h.session.project.Revision {
		t.Fatalf("workspace project = %+v", view.Project)
	}
	if len(view.Chains) != 2 || view.Chains[0].ID != "c_n_b" || view.Chains[1].ID != "c_n_c" {
		t.Fatalf("derived chains = %+v", view.Chains)
	}
	if len(view.Pending) != 1 || view.Pending[0].ID != pending.ID {
		t.Fatalf("pending summaries = %+v", view.Pending)
	}
	if err := h.service.AcquireRunLock(context.Background(), "r_workspace_lock", "n_topic", "n_a", []string{"n_topic", "n_a"}); err != nil {
		t.Fatal(err)
	}
	view, err = h.service.WorkspaceView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(view.LockedNodeIDs) != 2 || view.LockedNodeIDs[0] != "n_a" || view.LockedNodeIDs[1] != "n_topic" {
		t.Fatalf("project locked nodes = %+v", view.LockedNodeIDs)
	}
	if len(view.RunLocks) != 1 || view.RunLocks[0].RunID != "r_workspace_lock" || len(view.RunLocks[0].ChainPath) != 2 {
		t.Fatalf("workspace run locks = %+v", view.RunLocks)
	}
	if got := (*h.changes)[len(*h.changes)-1]; got.projectID != h.session.project.ID || got.reason != ChangeReasonRunLock {
		t.Fatalf("run lock event = %+v", got)
	}
}

func TestCreatePendingEnforcesPayloadAndTreeLimits(t *testing.T) {
	h := newPendingHarness(t, projectWithSecondTree())
	for i := 0; i < valueobject.MaxPendingPerTree; i++ {
		input := pendingInput(fmt.Sprintf("r_%d", i), "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, validScriptPayload())
		if _, err := h.service.CreatePending(context.Background(), input); err != nil {
			t.Fatalf("create pending %d: %v", i, err)
		}
	}
	tooMany := pendingInput("r_over", "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, validScriptPayload())
	if _, err := h.service.CreatePending(context.Background(), tooMany); !errors.Is(err, ErrPendingLimit) {
		t.Fatalf("51st pending error = %v, want PENDING_LIMIT_REACHED", err)
	}
	pendingList, err := h.service.ListPending(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingList) != valueobject.MaxPendingPerTree {
		t.Fatalf("pending count = %d", len(pendingList))
	}
	if _, err := h.service.DiscardPending(context.Background(), pendingList[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.CreatePending(context.Background(), tooMany); err != nil {
		t.Fatalf("released pending slot was not reusable: %v", err)
	}
	secondTree := pendingInput("r_second_tree", "n_x", []string{"n_topic2", "n_x"}, valueobject.OutputKindScript, validScriptPayload())
	if _, err := h.service.CreatePending(context.Background(), secondTree); err != nil {
		t.Fatalf("pending limit leaked across topic trees: %v", err)
	}
	oversized := pendingInput("r_large", "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, bytes.Repeat([]byte("x"), valueobject.MaxPendingPayloadSize+1))
	before := h.session.project.Revision
	if _, err := h.service.CreatePending(context.Background(), oversized); !errors.Is(err, ErrPendingPayloadTooLarge) {
		t.Fatalf("oversized payload error = %v", err)
	}
	if h.session.project.Revision != before {
		t.Fatal("oversized output changed project revision")
	}
}

func TestApplyPendingConflictsForceLockingAndStalePropagation(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	input := pendingInput("r_apply", "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, validScriptPayload())
	pending, err := h.service.CreatePending(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	h.hasher.hashes.ParamsHash = "params-v2"
	if _, err := h.service.ApplyPending(context.Background(), pending.ID, false); !errors.Is(err, ErrParamsChanged) {
		t.Fatalf("params mismatch error = %v, want PARAMS_CHANGED", err)
	}
	if h.session.project.Revision != 2 || h.node(t, "n_a").Locked {
		t.Fatal("conflicting apply changed project state")
	}
	h.hasher.hashes = valueobject.ContextHashes{ContextHash: "new-context", AncestorHash: "ancestor-v2", ParamsHash: "params-v1"}
	if _, err := h.service.ApplyPending(context.Background(), pending.ID, false); !errors.Is(err, ErrContextChanged) {
		t.Fatalf("ancestor mismatch error = %v, want CONTEXT_CHANGED", err)
	}
	applied, err := h.service.ApplyPending(context.Background(), pending.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Revision != 3 || !h.node(t, "n_a").Locked || h.node(t, "n_a").Stale || !h.node(t, "n_b").Stale {
		t.Fatalf("apply state: revision=%d node=%+v child=%+v", applied.Revision, h.node(t, "n_a"), h.node(t, "n_b"))
	}
	if len(h.node(t, "n_a").Rounds) != 1 || h.node(t, "n_a").Rounds[0].N != 1 || h.node(t, "n_a").Rounds[0].PendingID != pending.ID {
		t.Fatalf("applied rounds = %+v", h.node(t, "n_a").Rounds)
	}
	stored, err := h.pending.Load(context.Background(), pending.ID)
	if err != nil || stored.Status != valueobject.PendingApplied || stored.ArtifactID == "" {
		t.Fatalf("stored pending = %+v err=%v", stored, err)
	}
	if _, _, err := h.artifacts.Get(context.Background(), stored.ArtifactID); err != nil {
		t.Fatalf("applied artifact missing: %v", err)
	}
	if _, err := h.service.ApplyPending(context.Background(), pending.ID, true); !errors.Is(err, ErrPendingAlreadyApplied) {
		t.Fatalf("duplicate apply error = %v", err)
	}
	if _, err := h.service.DiscardPending(context.Background(), pending.ID); !errors.Is(err, ErrPendingAlreadyApplied) {
		t.Fatalf("discard applied error = %v", err)
	}
}

func TestForceCannotBypassInvalidOutputOrRunLock(t *testing.T) {
	t.Run("invalid schema", func(t *testing.T) {
		h := newPendingHarness(t, baseProject())
		input := pendingInput("r_invalid", "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, []byte(`{"title":"bad","scenes":[]}`))
		pending, err := h.service.CreatePending(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.service.ApplyPending(context.Background(), pending.ID, true); !errors.Is(err, ErrPendingInvalid) {
			t.Fatalf("force invalid apply error = %v, want PENDING_INVALID", err)
		}
	})
	t.Run("run lock", func(t *testing.T) {
		h := newPendingHarness(t, baseProject())
		input := pendingInput("r_locked", "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, validScriptPayload())
		pending, err := h.service.CreatePending(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		if err := h.service.AcquireRunLock(context.Background(), "r_active", "n_topic", "n_a", []string{"n_topic", "n_a"}); err != nil {
			t.Fatal(err)
		}
		if _, err := h.service.ApplyPending(context.Background(), pending.ID, true); !errors.Is(err, ErrRunningChainLocked) {
			t.Fatalf("force on run-locked path error = %v, want RUNNING_CHAIN_LOCKED", err)
		}
	})
}

func TestDiscardPendingIsIdempotent(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	input := pendingInput("r_discard", "n_a", []string{"n_topic", "n_a"}, valueobject.OutputKindScript, []byte("{"))
	pending, err := h.service.CreatePending(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	discarded, err := h.service.DiscardPending(context.Background(), pending.ID)
	if err != nil || discarded.Revision != 3 {
		t.Fatalf("discard project=%+v err=%v", discarded, err)
	}
	stored, err := h.pending.Load(context.Background(), pending.ID)
	if err != nil || stored.Status != valueobject.PendingDiscarded {
		t.Fatalf("discarded pending=%+v err=%v", stored, err)
	}
	changes := len(*h.changes)
	discarded, err = h.service.DiscardPending(context.Background(), pending.ID)
	if err != nil || discarded.Revision != 3 || len(*h.changes) != changes {
		t.Fatalf("repeated discard project=%+v err=%v changes=%d", discarded, err, len(*h.changes))
	}
	if _, err := h.service.ApplyPending(context.Background(), pending.ID, true); !errors.Is(err, ErrPendingNotApplicable) {
		t.Fatalf("apply discarded pending error = %v", err)
	}
}

func TestApplyTopicUsesRoundKindWhenArtifactMetadataWasFirstWrittenForAnotherKind(t *testing.T) {
	zero := 0
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_topic", Name: "选题工程", Revision: 1,
		ActiveNodeID: "n_topic", Nodes: []entity.Node{
			{ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "新选题", Params: entity.NodeParams{CandidateCount: 3}, Rounds: []entity.Round{}},
			{ID: "n_child", Type: valueobject.NodeTypePrompt, Title: "脚本", Parent: "n_topic", ParentCandidate: &zero, Params: entity.NodeParams{OutputKind: valueobject.OutputKindScript}, Rounds: []entity.Round{}},
		},
	}
	h := newPendingHarness(t, project)
	payload := []byte(`{"topics":[{"title":"a","angle":"a","hook":"a"},{"title":"b","angle":"b","hook":"b"},{"title":"c","angle":"c","hook":"c"}]}`)
	canonical, err := valueobject.CanonicalJSON(payload)
	if err != nil {
		t.Fatal(err)
	}
	first, err := h.artifacts.Put(context.Background(), canonical, entity.ArtifactMeta{
		Kind: valueobject.OutputKindScript, NodeID: "n_seed", CreatedAt: "2026-11-01T10:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	input := pendingInput("r_topic", "n_topic", []string{"n_topic"}, valueobject.OutputKindTopics, payload)
	pending, err := h.service.CreatePending(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := h.service.ApplyPending(context.Background(), pending.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if applied.Nodes[0].Rounds[0].ArtifactID != first.ID || applied.Nodes[0].Rounds[0].Kind != valueobject.OutputKindTopics {
		t.Fatalf("round did not retain authoritative kind and reused artifact: %+v", applied.Nodes[0].Rounds)
	}
	counter := NewArtifactCandidateCounter(&fakeArtifactProvider{store: h.artifacts})
	count, err := counter.Count(context.Background(), project.ID, applied.Nodes[0])
	if err != nil || count != 3 {
		t.Fatalf("candidate count=%d err=%v after cross-kind CAS reuse", count, err)
	}
}

func (h *pendingHarness) node(t *testing.T, id string) *entity.Node {
	t.Helper()
	for i := range h.session.project.Nodes {
		if h.session.project.Nodes[i].ID == id {
			return &h.session.project.Nodes[i]
		}
	}
	t.Fatalf("node %s not found", id)
	return nil
}
