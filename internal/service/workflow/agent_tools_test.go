package workflow

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

func scopedRun() port.RunScope {
	return port.RunScope{
		RunID: "r_mcp", ProjectID: "p_test", TreeID: "n_topic", NodeID: "n_b",
		ChainID: "c_n_b", ChainPath: []string{"n_topic", "n_candidate", "n_a", "n_b"},
		ContextHash: "context-v1", AncestorHash: "ancestor-v1", ParamsHash: "params-v1",
	}
}

func putRunArtifact(t *testing.T, h *pendingHarness, kind valueobject.OutputKind, nodeID string, payload []byte) entity.Artifact {
	t.Helper()
	artifact, err := h.artifacts.Put(context.Background(), payload, entity.ArtifactMeta{
		Kind: kind, NodeID: nodeID, CreatedAt: "2026-11-02T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	return artifact
}

func setNodeRounds(project *entity.Project, nodeID string, rounds ...entity.Round) {
	for i := range project.Nodes {
		if project.Nodes[i].ID == nodeID {
			project.Nodes[i].Rounds = append([]entity.Round(nil), rounds...)
			return
		}
	}
}

func setCandidateArtifact(project *entity.Project, artifactID string) {
	for i := range project.Nodes {
		if project.Nodes[i].Type == valueobject.NodeTypeCandidate && project.Nodes[i].CandidateSource != nil && project.Nodes[i].CandidateSource.TopicNodeID == "n_topic" {
			project.Nodes[i].CandidateSource.ArtifactID = artifactID
		}
	}
}

func appendNodeWithEdge(project *entity.Project, node entity.Node, sourceID string) {
	if node.WorkspaceRootID == "" {
		node.WorkspaceRootID = "n_topic"
	}
	project.Nodes = append(project.Nodes, node)
	if sourceID != "" {
		attachEdge(project, sourceID, node.ID)
	}
}

func TestRunGraphAndNodeExposeOnlyPath(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	setNodeRounds(&h.session.project, "n_a", entity.Round{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: "sha256:abc", PendingID: "p_old", RunID: "r_old"})
	scope := scopedRun()

	graph, err := h.service.ReadRunGraph(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != len(scope.ChainPath) {
		t.Fatalf("graph exposed %d nodes, want path length %d", len(graph.Nodes), len(scope.ChainPath))
	}
	for i, id := range scope.ChainPath {
		if graph.Nodes[i].ID != id {
			t.Fatalf("graph node %d = %s, want %s", i, graph.Nodes[i].ID, id)
		}
	}
	for _, node := range graph.Nodes {
		if node.ID == "n_c" || node.Title == "草稿" {
			t.Fatalf("graph leaked a sibling or draft: %+v", node)
		}
	}

	node, err := h.service.ReadRunNode(context.Background(), scope, "n_a")
	if err != nil {
		t.Fatal(err)
	}
	if node.ID != "n_a" || node.Params.Instruction != "A" || len(node.Rounds) != 1 || node.Rounds[0].ArtifactID != "sha256:abc" {
		t.Fatalf("node output = %+v", node)
	}
	if _, err := h.service.ReadRunNode(context.Background(), scope, "n_c"); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("sibling read error = %v, want CHAIN_SCOPE_DENIED", err)
	}

	wrongTree := scope
	wrongTree.TreeID = "n_topic2"
	if _, err := h.service.ReadRunGraph(context.Background(), wrongTree); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("wrong-tree graph error = %v, want CHAIN_SCOPE_DENIED", err)
	}
}

func TestDAGRunToolsExposeOnlySelectedFrozenInputs(t *testing.T) {
	project := rootlessBoundScriptProject()
	project.Nodes = append(project.Nodes, entity.Node{
		ID: "n_other_task", Type: valueobject.NodeTypePrompt, Title: "未选任务",
		Params: entity.NodeParams{Instruction: "不参与本次运行"}, Rounds: []entity.Round{},
	})
	project.Edges = append(project.Edges, entity.Edge{
		ID: "e_other_agent", SourceNodeID: "n_other_task", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection,
	})
	h := newPendingHarness(t, project)
	artifact := putRunArtifact(t, h, valueobject.OutputKindScript, "n_source", validScriptPayload())
	for i := range h.session.project.Nodes {
		if h.session.project.Nodes[i].ID == "n_source" {
			h.session.project.Nodes[i].ScriptVersions[0].ArtifactID = artifact.ID
		}
	}
	scope := beginReplyScopeWithInputs(t, h, "r_dag_tools", "n_agent", []string{"e_prompt_agent"})
	graph, err := h.service.ReadRunGraph(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(graph.Nodes) != len(scope.InputNodeIDs) || len(graph.Edges) != len(scope.ExecutionSnapshot.InputEdges) {
		t.Fatalf("DAG graph scope = %+v", graph)
	}
	for _, node := range graph.Nodes {
		if node.ID == "n_other_task" || node.ID == "n_output" {
			t.Fatalf("DAG graph leaked an unselected node: %+v", node)
		}
	}
	if _, err := h.service.ReadRunNode(context.Background(), scope, "n_other_task"); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("unselected node access error = %v", err)
	}
	input, err := h.service.ReadRunNode(context.Background(), scope, "n_source")
	if err != nil || input.LatestScriptVersion == nil || input.LatestScriptVersion.ArtifactID != artifact.ID {
		t.Fatalf("selected Script node = %+v err=%v", input, err)
	}
	canonical, err := valueobject.CanonicalJSON(validScriptPayload())
	if err != nil {
		t.Fatal(err)
	}
	read, err := h.service.ReadRunArtifact(context.Background(), scope, artifact.ID)
	if err != nil || read.Kind != valueobject.OutputKindScript || !bytes.Equal(read.Data, canonical) {
		t.Fatalf("selected Script artifact = %+v err=%v", read, err)
	}
}

func TestSnapshotRunToolsIgnoreLiveGraphAndUseFrozenArtifactAuthorization(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	appendNodeWithEdge(&h.session.project, entity.Node{
		ID: "n_output", Type: valueobject.NodeTypeScript, Title: "绑定输出",
	}, "n_b")
	for i := range h.session.project.Nodes {
		if h.session.project.Nodes[i].ID == "n_a" {
			h.session.project.Nodes[i].Rounds = []entity.Round{{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: "sha256:legacy-round"}}
		}
	}
	topics := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}]}`)
	artifact := putRunArtifact(t, h, valueobject.OutputKindTopics, "n_topic", topics)
	setCandidateArtifact(&h.session.project, artifact.ID)
	setNodeRounds(&h.session.project, "n_topic", entity.Round{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifact.ID})
	graph, err := entity.NewGraph(&h.session.project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRun("n_b")
	if err != nil {
		t.Fatal(err)
	}
	scope := port.RunScope{
		RunID: "r_snapshot", ProjectID: "p_test", TreeID: "n_topic", NodeID: "n_b",
		ChainID: "c_n_b", ChainPath: append([]string(nil), snapshot.ChainPath...),
		ContextHash: "context", AncestorHash: "ancestor", ParamsHash: "params",
		ExecutionSnapshot: snapshot,
	}

	for i := range h.session.project.Nodes {
		switch h.session.project.Nodes[i].ID {
		case "n_topic":
			h.session.project.Nodes[i].Title = "实时选题标题"
		case "n_candidate":
			h.session.project.Nodes[i].CandidateSource.Index = 1
		case "n_a":
			h.session.project.Nodes[i].Params.Instruction = "实时提示词"
		}
	}
	result, err := h.service.ReadRunGraph(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != len(scope.ChainPath) || result.Nodes[0].Title != "选题" || result.Nodes[1].CandidateSource == nil || result.Nodes[1].CandidateSource.Index != 0 {
		t.Fatalf("run graph did not use frozen path: %+v", result)
	}
	if result.OutputBinding == nil || !result.OutputBinding.Bound || result.OutputBinding.NodeID != "n_output" || result.OutputBinding.BaselineVersion != 0 {
		t.Fatalf("run graph omitted frozen output binding: %+v", result.OutputBinding)
	}
	node, err := h.service.ReadRunNode(context.Background(), scope, "n_a")
	if err != nil || node.Params.Instruction != "A" || len(node.Rounds) != 0 {
		t.Fatalf("run node did not use v2 snapshot fields: %+v err=%v", node, err)
	}
	view, err := h.service.ReadRunArtifact(context.Background(), scope, artifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(view.Data), `"title":"甲"`) || strings.Contains(string(view.Data), `"title":"乙"`) {
		t.Fatalf("artifact view did not use frozen candidate selection: %s", view.Data)
	}
	if _, err := h.service.ReadRunArtifact(context.Background(), scope, "sha256:unreferenced"); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("unreferenced snapshot artifact error = %v, want CHAIN_SCOPE_DENIED", err)
	}
}

func TestSnapshotRunArtifactUsesExactScriptVersion(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	topics := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}]}`)
	topicArtifact := putRunArtifact(t, h, valueobject.OutputKindTopics, "n_topic", topics)
	setCandidateArtifact(&h.session.project, topicArtifact.ID)
	setNodeRounds(&h.session.project, "n_topic", entity.Round{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: topicArtifact.ID})
	oldArtifact := putRunArtifact(t, h, valueobject.OutputKindScript, "n_script", []byte(`{"title":"旧版本","scenes":[{"narration":"旧旁白","duration":1}]}`))
	latestArtifact := putRunArtifact(t, h, valueobject.OutputKindScript, "n_script", []byte(`{"title":"快照版本","scenes":[{"narration":"快照旁白","duration":2}]}`))
	appendNodeWithEdge(&h.session.project, entity.Node{
		ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本",
		ScriptVersions: []entity.ScriptVersion{
			{N: 1, ArtifactID: oldArtifact.ID, RunID: "r_old"},
			{N: 2, ArtifactID: latestArtifact.ID, RunID: "r_latest"},
		},
	}, "n_b")
	appendNodeWithEdge(&h.session.project, entity.Node{ID: "n_prompt2", Type: valueobject.NodeTypePrompt, Title: "后续任务", Params: entity.NodeParams{Instruction: "后续"}}, "n_script")
	appendNodeWithEdge(&h.session.project, entity.Node{ID: "n_agent2", Type: valueobject.NodeTypeAgent, Title: "后续 Agent"}, "n_prompt2")
	graph, err := entity.NewGraph(&h.session.project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRun("n_agent2")
	if err != nil {
		t.Fatal(err)
	}
	scope := port.RunScope{
		RunID: "r_snapshot_script", ProjectID: "p_test", TreeID: "n_topic", NodeID: "n_agent2",
		ChainID: "c_n_agent2", ChainPath: append([]string(nil), snapshot.ChainPath...),
		ContextHash: "context", AncestorHash: "ancestor", ParamsHash: "params", ExecutionSnapshot: snapshot,
	}
	for i := range h.session.project.Nodes {
		if h.session.project.Nodes[i].ID == "n_script" {
			h.session.project.Nodes[i].ScriptVersions = nil
		}
	}
	view, err := h.service.ReadRunArtifact(context.Background(), scope, latestArtifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Kind != valueobject.OutputKindScript || !strings.Contains(string(view.Data), `"title":"快照版本"`) {
		t.Fatalf("snapshot script view = %s (%s)", view.Data, view.Kind)
	}
	if _, err := h.service.ReadRunArtifact(context.Background(), scope, oldArtifact.ID); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("unselected script version error = %v, want CHAIN_SCOPE_DENIED", err)
	}
}

func TestReadRunArtifactFiltersTopicCandidateAndUsesRoundKind(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	topics := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}]}`)
	topicArtifact := putRunArtifact(t, h, valueobject.OutputKindTopics, "n_topic", topics)
	setCandidateArtifact(&h.session.project, topicArtifact.ID)
	setNodeRounds(&h.session.project, "n_topic", entity.Round{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: topicArtifact.ID})

	script := validScriptPayload()
	scriptArtifact := putRunArtifact(t, h, valueobject.OutputKindScript, "n_a", script)
	setNodeRounds(&h.session.project, "n_a", entity.Round{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: scriptArtifact.ID})

	scope := scopedRun()
	view, err := h.service.ReadRunArtifact(context.Background(), scope, topicArtifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Kind != valueobject.OutputKindTopics || string(view.Data) != `{"angle":"角度甲","hook":"开头甲","title":"甲"}` {
		t.Fatalf("topic view = %s (%s)", view.Data, view.Kind)
	}
	for _, hidden := range []string{"乙", "丙"} {
		if strings.Contains(string(view.Data), hidden) {
			t.Fatalf("topic view leaked candidate %q: %s", hidden, view.Data)
		}
	}

	scriptView, err := h.service.ReadRunArtifact(context.Background(), scope, scriptArtifact.ID)
	if err != nil {
		t.Fatal(err)
	}
	if scriptView.Kind != valueobject.OutputKindScript || string(scriptView.Data) != `{"scenes":[{"duration":0,"narration":"旁白"}],"title":"脚本"}` {
		t.Fatalf("script view = %s (%s)", scriptView.Data, scriptView.Kind)
	}
	if _, err := h.service.ReadRunArtifact(context.Background(), scope, "sha256:"+strings.Repeat("f", 64)); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("unreferenced artifact error = %v, want CHAIN_SCOPE_DENIED", err)
	}
}

func TestReadRunArtifactFailsClosedForInvalidCandidateAndConflictingReferences(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	topics := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}]}`)
	artifact := putRunArtifact(t, h, valueobject.OutputKindTopics, "n_topic", topics)
	setCandidateArtifact(&h.session.project, artifact.ID)
	setNodeRounds(&h.session.project, "n_topic", entity.Round{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifact.ID})
	scope := scopedRun()

	h.session.project.Nodes[1].CandidateSource.Index = 5
	if _, err := h.service.ReadRunArtifact(context.Background(), scope, artifact.ID); !errors.Is(err, ErrCandidateInvalid) {
		t.Fatalf("invalid candidate error = %v, want CANDIDATE_INVALID", err)
	}
	h.session.project.Nodes[1].CandidateSource.Index = 0

	combined := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}],"title":"脚本","scenes":[{"narration":"旁白","duration":0}]}`)
	shared := putRunArtifact(t, h, valueobject.OutputKindTopics, "n_topic", combined)
	setCandidateArtifact(&h.session.project, shared.ID)
	setNodeRounds(&h.session.project, "n_topic", entity.Round{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: shared.ID})
	setNodeRounds(&h.session.project, "n_a", entity.Round{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: shared.ID})
	if _, err := h.service.ReadRunArtifact(context.Background(), scope, shared.ID); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("conflicting duplicate artifact error = %v, want CHAIN_SCOPE_DENIED", err)
	}
}

func TestSubmitRunReplyCommitsScriptAndDeduplicatesByRequestIdentity(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	scope := beginReplyScope(t, h, "r_reply", "n_b")
	payload := validScriptPayload()

	first, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, payload, "req_first")
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != valueobject.PendingApplied || first.RequestID != "req_first" || first.ResultNodeID == "" || len(first.CreatedNodeIDs) != 1 || first.CreatedNodeIDs[0] != first.ResultNodeID {
		t.Fatalf("applied reply = %+v", first)
	}
	if h.session.project.Revision != 2 {
		t.Fatalf("project revision = %d, want 2", h.session.project.Revision)
	}
	graph, err := entity.NewGraph(&h.session.project)
	if err != nil {
		t.Fatal(err)
	}
	resultNode, ok := graph.Node(first.ResultNodeID)
	outputEdge, hasOutputEdge := graph.EdgeBetween(scope.NodeID, first.ResultNodeID)
	if !ok || resultNode.Type != valueobject.NodeTypeScript || !hasOutputEdge || outputEdge.Kind != entity.EdgeKindConnection || len(resultNode.ScriptVersions) != 1 || resultNode.ScriptVersions[0].ArtifactID != first.ArtifactID {
		t.Fatalf("generated script node = %+v, ok=%t", resultNode, ok)
	}
	for _, other := range h.session.project.Nodes {
		if other.ID == resultNode.ID {
			continue
		}
		if resultNode.Position.X < other.Position.X+other.Size.Width+generatedNodeGap &&
			resultNode.Position.X+resultNode.Size.Width+generatedNodeGap > other.Position.X &&
			resultNode.Position.Y < other.Position.Y+other.Size.Height+generatedNodeGap &&
			resultNode.Position.Y+resultNode.Size.Height+generatedNodeGap > other.Position.Y {
			t.Fatalf("generated Script overlaps %s", other.ID)
		}
	}

	beforeNodes := len(h.session.project.Nodes)
	beforeChanges := len(*h.changes)
	repeated, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, payload, "req_first")
	if err != nil || repeated.ID != first.ID || h.session.project.Revision != 2 || len(h.session.project.Nodes) != beforeNodes || len(*h.changes) != beforeChanges {
		t.Fatalf("idempotent reply = %+v err=%v revision=%d nodes=%d", repeated, err, h.session.project.Revision, len(h.session.project.Nodes))
	}

	changed := []byte(`{"title":"别的脚本","scenes":[{"narration":"旁白","duration":0}]}`)
	if _, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, changed, "req_first"); !errors.Is(err, ErrRequestIDConflict) {
		t.Fatalf("same request id with different input error = %v, want REQUEST_ID_CONFLICT", err)
	}
	if _, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, payload, "req_second"); !errors.Is(err, ErrRunResultConflict) {
		t.Fatalf("second result error = %v, want RUN_RESULT_CONFLICT", err)
	}
}

func TestSubmitRunReplyPreservesInvalidPayloadAndRejectsWrongScope(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	scope := beginReplyScope(t, h, "r_invalid", "n_b")
	badScope := scope
	badScope.ProjectID = "p_other"
	if _, err := h.service.SubmitRunReply(context.Background(), badScope, valueobject.OutputKindScript, validScriptPayload(), "req_bad"); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("wrong project error = %v, want CHAIN_SCOPE_DENIED", err)
	}
	if h.session.project.Revision != 1 {
		t.Fatal("rejected scope changed project revision")
	}

	invalid := []byte(`{"title":"无效","scenes":[]}`)
	pending, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, invalid, "req_invalid")
	if err != nil {
		t.Fatal(err)
	}
	if pending.Status != valueobject.PendingInvalid {
		t.Fatalf("pending status = %s, want invalid", pending.Status)
	}
	detail, err := h.service.GetPending(context.Background(), pending.ID, h.session.project.ID)
	if err != nil || !bytes.Equal(detail.RawPayload, invalid) {
		t.Fatalf("stored payload = %s err=%v", detail.RawPayload, err)
	}
	if h.session.project.Revision != 2 {
		t.Fatalf("invalid result revision = %d, want 2", h.session.project.Revision)
	}
}

func TestSubmitRunReplySerializesConcurrentResults(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	scope := beginReplyScope(t, h, "r_concurrent", "n_b")
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, requestID := range []string{"req_a", "req_b"} {
		wg.Add(1)
		go func(requestID string) {
			defer wg.Done()
			<-start
			_, err := h.service.SubmitRunReply(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), requestID)
			results <- err
		}(requestID)
	}
	close(start)
	wg.Wait()
	close(results)
	created, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			created++
		case errors.Is(err, ErrRunResultConflict):
			rejected++
		default:
			t.Fatalf("concurrent reply error = %v", err)
		}
	}
	if created != 1 || rejected != 1 {
		t.Fatalf("concurrent reply results: created=%d rejected=%d", created, rejected)
	}
}

func beginReplyScope(t *testing.T, h *pendingHarness, runID, nodeID string) port.RunScope {
	return beginReplyScopeWithOptions(t, h, runID, nodeID, entity.RunStartOptions{})
}

func beginReplyScopeWithInputs(t *testing.T, h *pendingHarness, runID, nodeID string, inputEdgeIDs []string) port.RunScope {
	return beginReplyScopeWithOptions(t, h, runID, nodeID, entity.RunStartOptions{InputEdgeIDs: inputEdgeIDs})
}

func beginReplyScopeWithOptions(t *testing.T, h *pendingHarness, runID, nodeID string, options entity.RunStartOptions) port.RunScope {
	t.Helper()
	_, snapshot, err := h.service.BeginRunWithOptions(context.Background(), runID, nodeID, options, h.session.project.ID)
	if err != nil {
		t.Fatal(err)
	}
	return port.RunScope{
		RunID: runID, ProjectID: snapshot.ProjectID, TreeID: snapshot.TreeID, NodeID: snapshot.NodeID,
		ChainID: "c_" + snapshot.NodeID, ChainPath: append([]string(nil), snapshot.ChainPath...),
		InputNodeIDs: snapshot.ScopeNodeIDs(),
		ContextHash:  h.hasher.hashes.ContextHash, AncestorHash: h.hasher.hashes.AncestorHash,
		ParamsHash: h.hasher.hashes.ParamsHash, ExecutionSnapshot: snapshot,
	}
}
