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

func scopedPromptRun() port.RunScope {
	scope := scopedRun()
	scope.NodeID = "n_a"
	scope.ChainID = "c_n_a"
	scope.ChainPath = []string{"n_topic", "n_candidate", "n_a"}
	return scope
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
		if project.Nodes[i].Type == valueobject.NodeTypeCandidate && project.Nodes[i].Parent == "n_topic" {
			project.Nodes[i].CandidateSource.ArtifactID = artifactID
		}
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

func TestSnapshotRunToolsIgnoreLiveGraphAndUseFrozenArtifactAuthorization(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	h.session.project.Nodes = append(h.session.project.Nodes, entity.Node{
		ID: "n_output", Type: valueobject.NodeTypeScript, Title: "绑定输出", Parent: "n_b",
	})
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
	h.session.project.Nodes = append(h.session.project.Nodes,
		entity.Node{
			ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本",
			Parent: "n_b", ScriptVersions: []entity.ScriptVersion{
				{N: 1, ArtifactID: oldArtifact.ID, RunID: "r_old"},
				{N: 2, ArtifactID: latestArtifact.ID, RunID: "r_latest"},
			},
		},
		entity.Node{ID: "n_prompt2", Type: valueobject.NodeTypePrompt, Title: "后续任务", Parent: "n_script", Params: entity.NodeParams{Instruction: "后续"}},
		entity.Node{ID: "n_agent2", Type: valueobject.NodeTypeAgent, Title: "后续 Agent", Parent: "n_prompt2"},
	)
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

func TestCreateRunPendingBindsScopeAndSerializesConcurrentReplies(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	scope := scopedPromptRun()
	badScope := scope
	badScope.ProjectID = "p_other"
	if _, err := h.service.CreateRunPending(context.Background(), badScope, valueobject.OutputKindScript, validScriptPayload(), "req_bad"); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("wrong project error = %v, want CHAIN_SCOPE_DENIED", err)
	}
	if h.session.project.Revision != 1 {
		t.Fatal("rejected scope changed project revision")
	}

	invalid := []byte(`{"title":"无效","scenes":[]}`)
	pending, err := h.service.CreateRunPending(context.Background(), scope, valueobject.OutputKindScript, invalid, "req_first")
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

	h = newPendingHarness(t, baseProject())
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, requestID := range []string{"req_a", "req_b"} {
		wg.Add(1)
		go func(requestID string) {
			defer wg.Done()
			<-start
			_, err := h.service.CreateRunPending(context.Background(), scope, valueobject.OutputKindScript, validScriptPayload(), requestID)
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
		case errors.Is(err, ErrTurnPendingExists):
			rejected++
		default:
			t.Fatalf("concurrent reply error = %v", err)
		}
	}
	if created != 1 || rejected != 1 {
		t.Fatalf("concurrent reply results: created=%d rejected=%d", created, rejected)
	}
}
