package workflow

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

func rootlessBoundScriptProject() entity.Project {
	return entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "无 Topic 运行", Revision: 1, ActiveNodeID: "n_agent",
		Nodes: []entity.Node{
			{ID: "n_source", Type: valueobject.NodeTypeScript, Title: "输入脚本", ScriptVersions: []entity.ScriptVersion{{N: 1, ArtifactID: "sha256:source", RunID: "r_source"}}, Rounds: []entity.Round{}},
			{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "任务", Params: entity.NodeParams{Instruction: "根据输入脚本续写"}, Rounds: []entity.Round{}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", Rounds: []entity.Round{}},
			{ID: "n_output", Type: valueobject.NodeTypeScript, Title: "输出", ScriptVersions: []entity.ScriptVersion{{N: 1, ArtifactID: "sha256:output-v1", RunID: "r_old"}}, Rounds: []entity.Round{}},
		},
		Edges: []entity.Edge{
			{ID: "e_source_prompt", SourceNodeID: "n_source", TargetNodeID: "n_prompt", Kind: entity.EdgeKindConnection, ScriptVersion: 1},
			{ID: "e_prompt_agent", SourceNodeID: "n_prompt", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection},
			{ID: "e_agent_output", SourceNodeID: "n_agent", TargetNodeID: "n_output", Kind: entity.EdgeKindConnection},
		},
	}
}

func TestRootlessDAGReplyPersistsSnapshotAndConfirmsFrozenOutput(t *testing.T) {
	h := newPendingHarness(t, rootlessBoundScriptProject())
	scope := beginReplyScopeWithInputs(t, h, "r_rootless_dag", "n_agent", []string{"e_prompt_agent"})
	if scope.TreeID != "" || len(scope.ChainPath) != 0 || scope.ExecutionSnapshot.Version != 4 ||
		!slices.Equal(scope.InputNodeIDs, scope.ExecutionSnapshot.ScopeNodeIDs()) {
		t.Fatalf("rootless run scope = %+v", scope)
	}

	first, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_rootless")
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != valueobject.PendingPending || first.ExecutionSnapshot == nil || first.ExecutionSnapshot.Version != 4 ||
		len(first.ChainPath) != 0 || first.OutputBinding == nil || !first.OutputBinding.Bound {
		t.Fatalf("DAG reply pending = %+v", first)
	}
	stored, err := h.service.GetPending(context.Background(), first.ID, h.session.project.ID)
	if err != nil || stored.Pending.ExecutionSnapshot == nil || stored.Pending.ExecutionSnapshot.Version != 4 {
		t.Fatalf("persisted DAG snapshot = %+v err=%v", stored.Pending.ExecutionSnapshot, err)
	}
	revision := h.session.project.Revision
	retry, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), "req_rootless")
	if err != nil || retry.ID != first.ID || h.session.project.Revision != revision {
		t.Fatalf("idempotent DAG reply = %+v err=%v revision=%d", retry, err, h.session.project.Revision)
	}
	if err := h.service.ReleaseRunLock(context.Background(), scope.RunID); err != nil {
		t.Fatal(err)
	}

	applied, err := h.service.ApplyPending(context.Background(), first.ID, false, h.session.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	output := pendingReplyNode(t, applied, "n_output")
	if len(output.ScriptVersions) != 2 || output.ScriptVersions[1].N != 2 || output.ScriptVersions[1].RunID != scope.RunID {
		t.Fatalf("confirmed DAG output versions = %+v", output.ScriptVersions)
	}
}

func TestSubmitTopicsReplyPlacesCandidatesAroundResizedNodes(t *testing.T) {
	topicSize := entity.NodeSize{Width: 500, Height: 250}
	blockerSize := entity.NodeSize{Width: 900, Height: 800}
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "测试工程", Revision: 1, ActiveNodeID: "n_topic",
		Nodes: []entity.Node{
			{
				ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "选题", WorkspaceRootID: "n_topic",
				Position: entity.Position{X: 0, Y: 0}, Size: topicSize,
				Params: entity.NodeParams{Idea: "想法", CandidateCount: 3}, Rounds: []entity.Round{},
			},
			{
				ID: "n_blocker", Type: valueobject.NodeTypeTopic, Title: "调整过的选题", WorkspaceRootID: "n_blocker",
				Position: entity.Position{X: topicSize.Width + generatedNodeGap, Y: 0}, Size: blockerSize,
				Params: entity.NodeParams{Idea: "其他想法", CandidateCount: 3}, Rounds: []entity.Round{},
			},
		},
	}
	h := newPendingHarness(t, project)
	scope := beginReplyScope(t, h, "r_topics_layout", "n_topic")
	payload := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}]}`)

	reply, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindTopics, payload, "req_topics_layout")
	if err != nil {
		t.Fatal(err)
	}
	if len(reply.CreatedNodeIDs) != 3 {
		t.Fatalf("generated candidates = %d, want 3", len(reply.CreatedNodeIDs))
	}
	if h.session.project.Nodes[0].Position != project.Nodes[0].Position || h.session.project.Nodes[1].Position != project.Nodes[1].Position {
		t.Fatal("automatic placement moved an existing node")
	}

	allNodes := append([]entity.Node(nil), project.Nodes...)
	for _, id := range reply.CreatedNodeIDs {
		candidate := pendingReplyNode(t, h.session.project, id)
		if candidate.Type != valueobject.NodeTypeCandidate || candidate.Size != entity.DefaultNodeSize(valueobject.NodeTypeCandidate) {
			t.Fatalf("generated node = %+v", candidate)
		}
		for _, other := range allNodes {
			if candidate.Position.X < other.Position.X+other.Size.Width+generatedNodeGap &&
				candidate.Position.X+candidate.Size.Width+generatedNodeGap > other.Position.X &&
				candidate.Position.Y < other.Position.Y+other.Size.Height+generatedNodeGap &&
				candidate.Position.Y+candidate.Size.Height+generatedNodeGap > other.Position.Y {
				t.Fatalf("generated node %s overlaps %s", candidate.ID, other.ID)
			}
		}
		allNodes = append(allNodes, candidate)
	}
}

func TestTopicFirstRunAndRerunKeepCandidateBatchIdentity(t *testing.T) {
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_topic_batches", Name: "选题批次", Revision: 1, ActiveNodeID: "n_topic",
		Nodes: []entity.Node{{
			ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "选题", WorkspaceRootID: "n_topic",
			Position: entity.Position{X: 40, Y: 60}, Size: entity.DefaultNodeSize(valueobject.NodeTypeTopic),
			Params: entity.NodeParams{Idea: "夜间城市", CandidateCount: 3}, Rounds: []entity.Round{},
		}},
	}
	h := newPendingHarness(t, project)
	payload := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}]}`)

	firstScope := beginReplyScope(t, h, "r_topic_first", "n_topic")
	first, err := h.service.SubmitRunReply(context.Background(), firstScope, valueobject.OutputKindTopics, payload, "req_topic_batch")
	if err != nil || first.Status != valueobject.PendingApplied || len(first.CreatedNodeIDs) != 3 {
		t.Fatalf("first topic result = %+v err=%v", first, err)
	}
	revision := h.session.project.Revision
	retry, err := h.service.SubmitRunReply(context.Background(), firstScope, valueobject.OutputKindTopics, payload, "req_topic_batch")
	if err != nil || retry.ID != first.ID || h.session.project.Revision != revision || len(h.session.project.Nodes) != len(project.Nodes)+3 {
		t.Fatalf("first topic retry = %+v err=%v revision=%d", retry, err, h.session.project.Revision)
	}
	firstArtifactID := pendingReplyNode(t, h.session.project, "n_topic").Rounds[0].ArtifactID
	if err := h.service.ReleaseRunLock(context.Background(), firstScope.RunID); err != nil {
		t.Fatal(err)
	}

	secondScope := beginReplyScopeWithOptions(t, h, "r_topic_second", "n_topic", entity.RunStartOptions{TopicRerun: true})
	if secondScope.ExecutionSnapshot.TopicBatch == nil || secondScope.ExecutionSnapshot.TopicBatch.BaselineRound != 1 {
		t.Fatalf("topic rerun snapshot = %+v", secondScope.ExecutionSnapshot.TopicBatch)
	}
	second, err := h.service.SubmitRunReply(context.Background(), secondScope, valueobject.OutputKindTopics, payload, "req_topic_batch")
	if err != nil || second.Status != valueobject.PendingPending || second.ArtifactID != "" || len(second.CreatedNodeIDs) != 0 {
		t.Fatalf("topic rerun result = %+v err=%v", second, err)
	}
	revision = h.session.project.Revision
	secondRetry, err := h.service.SubmitRunReply(context.Background(), secondScope, valueobject.OutputKindTopics, payload, "req_topic_batch")
	if err != nil || secondRetry.ID != second.ID || h.session.project.Revision != revision {
		t.Fatalf("topic rerun retry = %+v err=%v revision=%d", secondRetry, err, h.session.project.Revision)
	}
	if err := h.service.ReleaseRunLock(context.Background(), secondScope.RunID); err != nil {
		t.Fatal(err)
	}

	applied, err := h.service.ApplyPending(context.Background(), second.ID, false, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := h.service.GetPending(context.Background(), second.ID, project.ID)
	if err != nil || stored.Pending.Status != valueobject.PendingApplied || stored.Pending.ArtifactID != firstArtifactID || len(stored.Pending.CreatedNodeIDs) != 3 {
		t.Fatalf("applied topic rerun = %+v err=%v", stored.Pending, err)
	}
	topic := pendingReplyNode(t, applied, "n_topic")
	if len(topic.Rounds) != 2 || topic.Rounds[0].ArtifactID != firstArtifactID || topic.Rounds[0].RunID != firstScope.RunID ||
		topic.Rounds[1].ArtifactID != firstArtifactID || topic.Rounds[1].RunID != secondScope.RunID {
		t.Fatalf("topic rounds = %+v", topic.Rounds)
	}

	seenIDs := make(map[string]bool)
	seenIndexes := map[int]map[int]bool{1: {}, 2: {}}
	batchCounts := map[int]int{}
	occupied := []entity.Node{topic}
	for _, candidate := range applied.Nodes {
		if candidate.Type != valueobject.NodeTypeCandidate || candidate.CandidateSource == nil || candidate.CandidateSource.TopicNodeID != topic.ID {
			continue
		}
		source := candidate.CandidateSource
		wantRunID := map[int]string{1: firstScope.RunID, 2: secondScope.RunID}[source.Round]
		if wantRunID == "" || source.RunID != wantRunID || source.ArtifactID != firstArtifactID || source.Index < 0 || source.Index >= 3 {
			t.Fatalf("candidate batch source = %+v", source)
		}
		if seenIndexes[source.Round][source.Index] || seenIDs[candidate.ID] {
			t.Fatalf("duplicate candidate identity: node=%+v", candidate)
		}
		seenIndexes[source.Round][source.Index] = true
		seenIDs[candidate.ID] = true
		batchCounts[source.Round]++
		if !generatedNodePositionClear(occupied, candidate.Position, candidate.Size) {
			t.Fatalf("candidate %s overlaps a prior batch node", candidate.ID)
		}
		occupied = append(occupied, candidate)
	}
	if batchCounts[1] != 3 || batchCounts[2] != 3 {
		t.Fatalf("candidate batch counts = %+v", batchCounts)
	}
	seenEdges := make(map[string]bool)
	edgeCount := 0
	for _, edge := range applied.Edges {
		if edge.SourceNodeID != topic.ID || edge.Kind != entity.EdgeKindCandidateSource {
			continue
		}
		edgeCount++
		if seenEdges[edge.ID] || !seenIDs[edge.TargetNodeID] {
			t.Fatalf("candidate source edge = %+v", edge)
		}
		seenEdges[edge.ID] = true
	}
	if edgeCount != 6 {
		t.Fatalf("candidate source edge count = %d, want 6", edgeCount)
	}
}
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
	appendNodeWithEdge(&project, entity.Node{ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本", ScriptVersions: versions, Rounds: []entity.Round{}}, "n_b")
	appendNodeWithEdge(&project, entity.Node{ID: "n_after_script", Type: valueobject.NodeTypePrompt, Title: "后续提示词", Params: entity.NodeParams{Instruction: "后续任务"}, Rounds: []entity.Round{}}, "n_script")
	appendNodeWithEdge(&project, entity.Node{ID: "n_after_agent", Type: valueobject.NodeTypeAgent, Title: "后续 Agent", Rounds: []entity.Round{}}, "n_after_script")
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
