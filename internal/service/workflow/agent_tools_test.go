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
		ChainID: "c_n_b", ChainPath: []string{"n_topic", "n_a", "n_b"},
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

func TestRunGraphAndNodeExposeOnlyPath(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	h.session.project.Nodes[1].Rounds = []entity.Round{{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: "sha256:abc", PendingID: "p_old", RunID: "r_old"}}
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

func TestReadRunArtifactFiltersTopicCandidateAndUsesRoundKind(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	topics := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}]}`)
	topicArtifact := putRunArtifact(t, h, valueobject.OutputKindTopics, "n_topic", topics)
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
	setNodeRounds(&h.session.project, "n_topic", entity.Round{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifact.ID})
	scope := scopedRun()

	*h.session.project.Nodes[1].ParentCandidate = 5
	if _, err := h.service.ReadRunArtifact(context.Background(), scope, artifact.ID); !errors.Is(err, ErrCandidateInvalid) {
		t.Fatalf("invalid candidate error = %v, want CANDIDATE_INVALID", err)
	}
	*h.session.project.Nodes[1].ParentCandidate = 0

	combined := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}],"title":"脚本","scenes":[{"narration":"旁白","duration":0}]}`)
	shared := putRunArtifact(t, h, valueobject.OutputKindTopics, "n_topic", combined)
	setNodeRounds(&h.session.project, "n_topic", entity.Round{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: shared.ID})
	setNodeRounds(&h.session.project, "n_a", entity.Round{N: 1, Kind: valueobject.OutputKindScript, ArtifactID: shared.ID})
	if _, err := h.service.ReadRunArtifact(context.Background(), scope, shared.ID); !errors.Is(err, port.ErrChainScopeDenied) {
		t.Fatalf("conflicting duplicate artifact error = %v, want CHAIN_SCOPE_DENIED", err)
	}
}

func TestCreateRunPendingBindsScopeAndSerializesConcurrentReplies(t *testing.T) {
	h := newPendingHarness(t, baseProject())
	scope := scopedRun()
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
