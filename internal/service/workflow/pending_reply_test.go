package workflow

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

func TestSubmitRunReplyAutomaticallyWritesFirstVersionToBoundEmptyScript(t *testing.T) {
	h := newBoundScriptReplyHarness(t, 0)
	scope := beginReplyScope(t, h, "r_empty_script", "n_b")

	reply, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_empty")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Status != valueobject.PendingApplied || reply.ResultNodeID != "n_script" || len(reply.CreatedNodeIDs) != 0 || reply.ArtifactID == "" {
		t.Fatalf("empty Script reply = %+v", reply)
	}
	output := pendingReplyNode(t, h.session.project, "n_script")
	if len(output.ScriptVersions) != 1 || output.ScriptVersions[0].N != 1 || output.ScriptVersions[0].ArtifactID != reply.ArtifactID || output.ScriptVersions[0].RunID != scope.RunID {
		t.Fatalf("first Script version = %+v", output.ScriptVersions)
	}
	if !pendingReplyNode(t, h.session.project, "n_after_script").Stale || !pendingReplyNode(t, h.session.project, "n_after_agent").Stale {
		t.Fatal("writing the first Script version did not stale downstream nodes")
	}
}

func TestSubmitRunReplyRequiresConfirmationToAppendExistingScriptVersion(t *testing.T) {
	h := newBoundScriptReplyHarness(t, 1)
	scope := beginReplyScope(t, h, "r_append_script", "n_b")
	before := pendingReplyNode(t, h.session.project, "n_script")

	reply, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_append")
	if err != nil {
		t.Fatal(err)
	}
	if reply.Status != valueobject.PendingPending || reply.ResultNodeID != before.ID || reply.ArtifactID != "" || reply.OutputBinding == nil ||
		!reply.OutputBinding.Bound || reply.OutputBinding.BaselineVersion != 1 || reply.OutputBinding.BaselineArtifactID != before.ScriptVersions[0].ArtifactID {
		t.Fatalf("existing Script reply = %+v", reply)
	}
	if got := pendingReplyNode(t, h.session.project, before.ID).ScriptVersions; len(got) != 1 || got[0] != before.ScriptVersions[0] {
		t.Fatalf("submission changed existing versions: %+v", got)
	}
	if err := h.service.ReleaseRunLock(context.Background(), scope.RunID); err != nil {
		t.Fatal(err)
	}

	applied, err := h.service.ApplyPending(context.Background(), reply.ID, false, h.session.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	output := pendingReplyNode(t, applied, before.ID)
	if len(output.ScriptVersions) != 2 || output.ScriptVersions[0] != before.ScriptVersions[0] ||
		output.ScriptVersions[1].N != 2 || output.ScriptVersions[1].RunID != scope.RunID {
		t.Fatalf("confirmed Script versions = %+v", output.ScriptVersions)
	}
	if !pendingReplyNode(t, applied, "n_after_script").Stale || !pendingReplyNode(t, applied, "n_after_agent").Stale {
		t.Fatal("appending a Script version did not stale downstream nodes")
	}
	stored, err := h.service.GetPending(context.Background(), reply.ID, h.session.project.ID)
	if err != nil || stored.Pending.Status != valueobject.PendingApplied || stored.Pending.ArtifactID != output.ScriptVersions[1].ArtifactID {
		t.Fatalf("applied pending = %+v err=%v", stored.Pending, err)
	}
}

func TestDiscardBoundScriptReplyLeavesExistingVersionsUnchanged(t *testing.T) {
	h := newBoundScriptReplyHarness(t, 1)
	scope := beginReplyScope(t, h, "r_discard_script", "n_b")
	before := pendingReplyNode(t, h.session.project, "n_script").ScriptVersions
	reply, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_discard")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.ReleaseRunLock(context.Background(), scope.RunID); err != nil {
		t.Fatal(err)
	}
	project, err := h.service.DiscardPending(context.Background(), reply.ID, h.session.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	output := pendingReplyNode(t, project, "n_script")
	if len(output.ScriptVersions) != len(before) || output.ScriptVersions[0] != before[0] {
		t.Fatalf("discard changed existing versions: %+v", output.ScriptVersions)
	}
	stored, err := h.service.GetPending(context.Background(), reply.ID, h.session.project.ID)
	if err != nil || stored.Pending.Status != valueobject.PendingDiscarded || stored.Pending.ArtifactID != "" {
		t.Fatalf("discarded pending = %+v err=%v", stored.Pending, err)
	}
}

func TestBoundScriptPendingChecksParamsAndAncestorHashes(t *testing.T) {
	h := newBoundScriptReplyHarness(t, 1)
	scope := beginReplyScope(t, h, "r_stale_script", "n_b")
	reply, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_stale")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.ReleaseRunLock(context.Background(), scope.RunID); err != nil {
		t.Fatal(err)
	}
	revision := h.session.project.Revision

	h.hasher.hashes.ParamsHash = "params-v2"
	if _, err := h.service.ApplyPending(context.Background(), reply.ID, false, h.session.project.ID); !errors.Is(err, ErrParamsChanged) {
		t.Fatalf("params mismatch error = %v, want PARAMS_CHANGED", err)
	}
	h.hasher.hashes.ParamsHash = scope.ParamsHash
	h.hasher.hashes.AncestorHash = "ancestor-v2"
	if _, err := h.service.ApplyPending(context.Background(), reply.ID, false, h.session.project.ID); !errors.Is(err, ErrContextChanged) {
		t.Fatalf("ancestor mismatch error = %v, want CONTEXT_CHANGED", err)
	}
	if h.session.project.Revision != revision || len(pendingReplyNode(t, h.session.project, "n_script").ScriptVersions) != 1 {
		t.Fatal("stale confirmation changed the project")
	}

	applied, err := h.service.ApplyPending(context.Background(), reply.ID, true, h.session.project.ID)
	if err != nil {
		t.Fatalf("force confirmation after input changes: %v", err)
	}
	if len(pendingReplyNode(t, applied, "n_script").ScriptVersions) != 2 {
		t.Fatal("force confirmation did not append a Script version")
	}
}

func TestForceCannotBypassBoundScriptVersionConflict(t *testing.T) {
	h := newBoundScriptReplyHarness(t, 1)
	scope := beginReplyScope(t, h, "r_conflict_script", "n_b")
	reply, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_conflict")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.service.ReleaseRunLock(context.Background(), scope.RunID); err != nil {
		t.Fatal(err)
	}
	for i := range h.session.project.Nodes {
		if h.session.project.Nodes[i].ID == "n_script" {
			h.session.project.Nodes[i].ScriptVersions = append(h.session.project.Nodes[i].ScriptVersions, entity.ScriptVersion{
				N: 2, ArtifactID: "sha256:external-version", RunID: "r_external",
			})
		}
	}
	if _, err := h.service.ApplyPending(context.Background(), reply.ID, true, h.session.project.ID); !errors.Is(err, ErrOutputVersionConflict) {
		t.Fatalf("force apply conflict error = %v, want OUTPUT_VERSION_CONFLICT", err)
	}
	output := pendingReplyNode(t, h.session.project, "n_script")
	if len(output.ScriptVersions) != 2 || output.ScriptVersions[1].RunID != "r_external" || h.session.project.Revision != 2 {
		t.Fatalf("version conflict changed project: versions=%+v revision=%d", output.ScriptVersions, h.session.project.Revision)
	}
	stored, err := h.service.GetPending(context.Background(), reply.ID, h.session.project.ID)
	if err != nil || stored.Pending.Status != valueobject.PendingPending {
		t.Fatalf("conflicted pending = %+v err=%v", stored.Pending, err)
	}
}

func TestSubmitRunReplyWithoutRequestIDIsIdempotent(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	scope := beginReplyScope(t, h, "r_no_request_id", "n_b")
	payload := validScriptPayload()
	first, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, payload, "")
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision, beforeNodes := h.session.project.Revision, len(h.session.project.Nodes)
	second, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, payload, "")
	if err != nil || second.ID != first.ID || h.session.project.Revision != beforeRevision || len(h.session.project.Nodes) != beforeNodes {
		t.Fatalf("no-request-id retry = %+v err=%v revision=%d nodes=%d", second, err, h.session.project.Revision, len(h.session.project.Nodes))
	}
}

func TestSubmitRunReplyRejectsExpiredRunAndChangedSnapshot(t *testing.T) {
	t.Run("expired run", func(t *testing.T) {
		h := newPendingHarness(t, baseProject())
		scope := beginReplyScope(t, h, "r_expired_reply", "n_b")
		if err := h.service.ReleaseRunLock(context.Background(), scope.RunID); err != nil {
			t.Fatal(err)
		}
		if _, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_expired"); !errors.Is(err, port.ErrTurnNotActive) {
			t.Fatalf("expired run error = %v, want TURN_NOT_ACTIVE", err)
		}
		if h.session.project.Revision != 1 || len(h.session.project.Nodes) != len(baseProject().Nodes) {
			t.Fatal("expired reply changed the graph")
		}
	})

	t.Run("changed path snapshot", func(t *testing.T) {
		h := newPendingHarness(t, baseProject())
		scope := beginReplyScope(t, h, "r_changed_reply", "n_b")
		for i := range h.session.project.Nodes {
			if h.session.project.Nodes[i].ID == "n_a" {
				h.session.project.Nodes[i].Params.Instruction = "已变化"
			}
		}
		if _, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_changed"); !errors.Is(err, port.ErrChainScopeDenied) {
			t.Fatalf("changed snapshot error = %v, want CHAIN_SCOPE_DENIED", err)
		}
		if h.session.project.Revision != 1 || len(h.session.project.Nodes) != len(baseProject().Nodes) {
			t.Fatal("changed snapshot reply changed the graph")
		}
	})
}

func TestSubmitRunReplyCommitFailuresKeepRecoveryState(t *testing.T) {
	injectedErr := errors.New("injected commit failure")
	stages := []string{"intent", "artifact", "project", "pending-index", "journal-delete"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			h := newPendingHarness(t, baseProject())
			scope := beginReplyScope(t, h, "r_fail", "n_b")
			*h.changes = nil
			pendingStore := &replyFaultPendingStore{PendingStore: h.pending}
			h.service.opts.Pending = pendingStoreProvider{store: pendingStore}
			switch stage {
			case "intent":
				pendingStore.saveTransactionErr = injectedErr
			case "artifact":
				h.service.opts.Artifacts = &fakeArtifactProvider{store: replyFaultArtifactStore{ArtifactStore: h.artifacts, putErr: injectedErr}}
			case "project":
				h.session.saveErr = injectedErr
			case "pending-index":
				pendingStore.saveErr = injectedErr
			case "journal-delete":
				pendingStore.deleteTransactionErr = injectedErr
			}

			if _, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_failure"); !errors.Is(err, injectedErr) {
				t.Fatalf("reply error = %v, want injected failure", err)
			}
			items, err := h.pending.List(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			_, transactionFound, err := h.pending.LoadTransaction(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			expectedRevision, expectedNodes := int64(1), len(baseProject().Nodes)
			expectedPending, expectedTransaction := false, stage != "intent"
			switch stage {
			case "pending-index", "journal-delete":
				expectedRevision = 2
				expectedNodes++
			}
			if stage == "journal-delete" {
				expectedPending = true
			}
			if h.session.project.Revision != expectedRevision || len(h.session.project.Nodes) != expectedNodes || len(items) != boolCount(expectedPending) || transactionFound != expectedTransaction {
				t.Fatalf("stage=%s revision=%d nodes=%d pending=%d transaction=%t", stage, h.session.project.Revision, len(h.session.project.Nodes), len(items), transactionFound)
			}
			wantChange := stage == "pending-index" || stage == "journal-delete"
			if len(*h.changes) != boolCount(wantChange) {
				t.Fatalf("stage=%s change notifications=%d", stage, len(*h.changes))
			}
			if wantChange {
				lock := h.service.locks["r_fail"]
				if (*h.changes)[0].revision != 2 || lock.ProjectRevision != 2 {
					t.Fatalf("stage=%s change=%+v lock revision=%d", stage, (*h.changes)[0], lock.ProjectRevision)
				}
			}
			if stage == "project" {
				transaction, found, err := h.pending.LoadTransaction(context.Background())
				if err != nil || !found {
					t.Fatalf("project-save failure lost journal: found=%t err=%v", found, err)
				}
				if exists, err := h.artifacts.Exists(context.Background(), transaction.Pending.ArtifactID); err != nil || !exists {
					t.Fatalf("safe orphan artifact missing: exists=%t err=%v", exists, err)
				}
			}
		})
	}
}

type replyFaultPendingStore struct {
	port.PendingStore
	saveTransactionErr   error
	saveErr              error
	deleteTransactionErr error
}

func (s *replyFaultPendingStore) SaveTransaction(ctx context.Context, transaction entity.PendingTransaction) error {
	if s.saveTransactionErr != nil {
		return s.saveTransactionErr
	}
	return s.PendingStore.SaveTransaction(ctx, transaction)
}

func (s *replyFaultPendingStore) Save(ctx context.Context, pending entity.Pending, payload []byte) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	return s.PendingStore.Save(ctx, pending, payload)
}

func (s *replyFaultPendingStore) DeleteTransaction(ctx context.Context) error {
	if s.deleteTransactionErr != nil {
		return s.deleteTransactionErr
	}
	return s.PendingStore.DeleteTransaction(ctx)
}

type replyFaultArtifactStore struct {
	port.ArtifactStore
	putErr error
}

func (s replyFaultArtifactStore) Put(ctx context.Context, payload []byte, meta entity.ArtifactMeta) (entity.Artifact, error) {
	if s.putErr != nil {
		return entity.Artifact{}, s.putErr
	}
	return s.ArtifactStore.Put(ctx, payload, meta)
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func newBoundScriptReplyHarness(t *testing.T, versionCount int) *pendingHarness {
	t.Helper()
	project := baseProject()
	versions := make([]entity.ScriptVersion, versionCount)
	for i := range versions {
		versions[i] = entity.ScriptVersion{N: i + 1, ArtifactID: fmt.Sprintf("sha256:existing-%d", i+1), RunID: fmt.Sprintf("r_existing_%d", i+1)}
	}
	project.Nodes = append(project.Nodes,
		entity.Node{ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本", Parent: "n_b", ScriptVersions: versions, Rounds: []entity.Round{}},
		entity.Node{ID: "n_after_script", Type: valueobject.NodeTypePrompt, Title: "后续提示词", Parent: "n_script", Params: entity.NodeParams{Instruction: "后续任务"}, Rounds: []entity.Round{}},
		entity.Node{ID: "n_after_agent", Type: valueobject.NodeTypeAgent, Title: "后续 Agent", Parent: "n_after_script", Rounds: []entity.Round{}},
	)
	return newPendingHarness(t, project)
}

func pendingReplyNode(t *testing.T, project entity.Project, id string) entity.Node {
	t.Helper()
	for _, node := range project.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("node %s not found", id)
	return entity.Node{}
}
