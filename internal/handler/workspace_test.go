package handler

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/persistence"
	"audio-ui/internal/service/project"
	runservice "audio-ui/internal/service/run"
	"audio-ui/internal/service/workflow"
)

type testWorkspaceRevision struct {
	mu        sync.Mutex
	revision  uint64
	projectID string
	unstable  bool
	reads     int
}

func (r *testWorkspaceRevision) Read() (uint64, string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if r.unstable {
		r.revision++
	}
	return r.revision, r.projectID
}

func (r *testWorkspaceRevision) projectChanged(id string, _ int64) {
	r.mu.Lock()
	r.revision++
	r.projectID = id
	r.mu.Unlock()
}

func (r *testWorkspaceRevision) workflowChanged(id string, _ int64, _ string) {
	r.mu.Lock()
	if id == r.projectID {
		r.revision++
	}
	r.mu.Unlock()
}

func TestGraphMutationReceiptMatchesAuthoritativeSnapshot(t *testing.T) {
	service, workflowService, revision, layout := newWorkspaceFixture(t, true)
	activeProject, err := service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	workflowHandler := NewWorkflowService(workflowService)
	receipt, err := workflowHandler.AddNode(AddNodeRequest{
		ProjectID: activeProject.ID, Type: "topic", Title: "新选题",
		Position: entity.Position{X: 535, Y: 362}, Params: entity.NodeParams{},
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := runservice.New(runservice.Options{Projects: service, RunStores: persistence.NewRunStoreProvider(layout)})
	workspaceHandler := NewWorkspaceService(workflowService, manager, revision)
	result, err := workspaceHandler.GetWorkspaceSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Project == nil || result.Snapshot.Project.ID != receipt.ProjectID || result.Snapshot.Project.Revision < receipt.ProjectRevision {
		t.Fatalf("receipt/snapshot project mismatch: receipt=%+v project=%+v", receipt, result.Snapshot.Project)
	}
	var created *entity.Node
	for i := range result.Snapshot.Nodes {
		if result.Snapshot.Nodes[i].ID == receipt.NodeID {
			created = &result.Snapshot.Nodes[i]
			break
		}
	}
	if created == nil || created.Type != valueobject.NodeTypeTopic || created.Position != (entity.Position{X: 535, Y: 362}) {
		t.Fatalf("receipt node missing or differs from snapshot: receipt=%+v node=%+v", receipt, created)
	}
}

func TestGetWorkspaceSnapshotReturnsSummariesDerivedChainsAndLocks(t *testing.T) {
	service, workflowService, revision, layout := newWorkspaceFixture(t, true)
	projectValue, err := service.New(context.Background(), "工作台")
	if err != nil {
		t.Fatal(err)
	}
	rootProject, err := workflowService.AddNode(context.Background(), workflow.AddNodeInput{
		Type: valueobject.NodeTypeTopic, Title: "主题", Position: entity.Position{X: 1, Y: 2},
		Params: entity.NodeParams{Idea: "ideas", CandidateCount: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	rootID := rootProject.Nodes[0].ID
	seededProject, err := service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	firstID, secondID := "n_prompt_a", "n_prompt_b"
	candidateA, candidateB := "n_candidate_a", "n_candidate_b"
	seededProject.Nodes[0].Rounds = []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:topics-a"}}
	seededProject.Nodes = append(seededProject.Nodes,
		entity.Node{
			ID: candidateA, Type: valueobject.NodeTypeCandidate, Title: "候选甲", Parent: rootID,
			Size:            entity.DefaultNodeSize(valueobject.NodeTypeCandidate),
			CandidateSource: &entity.CandidateSource{TopicNodeID: rootID, ArtifactID: "sha256:topics-a", Index: 0}, Rounds: []entity.Round{},
		},
		entity.Node{
			ID: firstID, Type: valueobject.NodeTypePrompt, Title: "分支甲", Parent: candidateA,
			Position: entity.Position{X: 3, Y: 4}, Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "draft"}, Rounds: []entity.Round{},
		},
		entity.Node{
			ID: candidateB, Type: valueobject.NodeTypeCandidate, Title: "候选乙", Parent: rootID,
			Size:            entity.DefaultNodeSize(valueobject.NodeTypeCandidate),
			CandidateSource: &entity.CandidateSource{TopicNodeID: rootID, ArtifactID: "sha256:topics-a", Index: 1}, Rounds: []entity.Round{},
		},
		entity.Node{
			ID: secondID, Type: valueobject.NodeTypePrompt, Title: "分支乙", Parent: candidateB,
			Position: entity.Position{X: 5, Y: 6}, Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Params: entity.NodeParams{Instruction: "draft"}, Rounds: []entity.Round{},
		},
	)
	seededProject.Revision++
	if _, err := service.Save(context.Background(), seededProject); err != nil {
		t.Fatal(err)
	}
	pending, err := workflowService.CreatePending(context.Background(), workflow.CreatePendingInput{
		RunID: "r_workspace", ProjectID: projectValue.ID, TreeID: rootID, ChainID: "c_" + firstID,
		ChainPath: []string{rootID, candidateA, firstID}, NodeID: firstID, Kind: valueobject.OutputKindScript,
		ContextHash: "context", AncestorHash: "ancestor", ParamsHash: "params",
		Payload: []byte(`{"title":"公开摘要","scenes":[{"narration":"private payload","duration":0}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := persistence.NewRunStoreProvider(layout).ForProject(context.Background(), projectValue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(context.Background(), entity.RunRecord{RunSummary: entity.RunSummary{
		ID: "r_history", TreeID: rootID, NodeID: firstID, ChainID: "c_" + firstID,
		ChainPath: []string{rootID, candidateA, firstID}, Status: valueobject.RunSucceeded,
		ContextHash: "context", EstimatedTokens: 42, StartedAt: "2026-11-03T00:00:00Z", UsageReported: false,
	}, Transcript: []valueobject.TranscriptItem{{ID: "t1", Text: "transcript must stay out"}}}); err != nil {
		t.Fatal(err)
	}
	manager := runservice.New(runservice.Options{Projects: service, RunStores: persistence.NewRunStoreProvider(layout)})
	handler := NewWorkspaceService(workflowService, manager, revision)
	pendingResult, err := handler.GetWorkspaceSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(pendingResult.Snapshot.Chains) != 2 || pendingResult.Snapshot.Chains[0].Status != "pending" || pendingResult.Snapshot.Chains[1].Status != "idle" {
		t.Fatalf("pending chain status = %+v", pendingResult.Snapshot.Chains)
	}
	if err := workflowService.AcquireRunLock(context.Background(), "r_lock", rootID, firstID, []string{rootID, candidateA, firstID}); err != nil {
		t.Fatal(err)
	}
	result, err := handler.GetWorkspaceSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if result.Snapshot.Project == nil || result.Snapshot.Project.ID != projectValue.ID || result.Snapshot.Project.Revision != seededProject.Revision+1 {
		t.Fatalf("snapshot project = %+v, revision=%d", result.Snapshot.Project, result.Revision)
	}
	if len(result.Snapshot.Nodes) != 5 || len(result.Snapshot.Chains) != 2 {
		t.Fatalf("nodes/chains = %d/%+v", len(result.Snapshot.Nodes), result.Snapshot.Chains)
	}
	if result.Snapshot.Chains[0].ID != "c_"+firstID || result.Snapshot.Chains[0].Status != "running" || result.Snapshot.Chains[1].ID != "c_"+secondID {
		t.Fatalf("derived chains = %+v", result.Snapshot.Chains)
	}
	if len(result.Snapshot.Pending) != 1 || result.Snapshot.Pending[0].ID != pending.ID || result.Snapshot.Pending[0].Summary != "公开摘要" {
		t.Fatalf("pending summaries = %+v", result.Snapshot.Pending)
	}
	if len(result.Snapshot.Runs) != 2 || result.Snapshot.Runs[0].ID != "r_history" || result.Snapshot.Runs[0].ActualInputTokens != nil {
		t.Fatalf("run summaries = %+v", result.Snapshot.Runs)
	}
	if result.Snapshot.Runs[1].ID != "r_lock" || result.Snapshot.Runs[1].Status != valueobject.RunStarting {
		t.Fatalf("starting lock summary = %+v", result.Snapshot.Runs)
	}
	locked := map[string]bool{}
	for _, id := range result.Snapshot.LockedNodeIDs {
		locked[id] = true
	}
	if len(result.Snapshot.LockedNodeIDs) != 3 || !locked[firstID] || !locked[candidateA] || !locked[rootID] {
		t.Fatalf("locked nodes = %+v", result.Snapshot.LockedNodeIDs)
	}
	encoded, err := json.Marshal(result.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || containsAny(string(encoded), "private payload", "transcript must stay out", "rawPayload", "pending/") {
		t.Fatalf("workspace snapshot contains a detail body: %s", encoded)
	}
}

func TestMakeWorkspaceSnapshotPreservesFrozenPendingTargetWithoutPayload(t *testing.T) {
	binding := &entity.RunOutputBinding{
		Bound: true, NodeID: "n_script", ParentNodeID: "n_agent", BaselineVersion: 2,
		BaselineArtifactID: "sha256:baseline",
	}
	view := workflow.WorkspaceView{
		Project: entity.Project{ID: "p_test", Name: "工程"},
		Pending: []entity.Pending{{
			ID: "pending_script", NodeID: "n_agent", RunID: "r_agent", ChainID: "c_n_agent",
			Kind: valueobject.OutputKindScript, Status: valueobject.PendingPending,
			OutputBinding: binding, ResultNodeID: "n_script", Payload: "private payload",
		}},
	}
	snapshot := makeWorkspaceSnapshot(view, nil)
	if len(snapshot.Pending) != 1 {
		t.Fatalf("pending summaries = %+v", snapshot.Pending)
	}
	summary := snapshot.Pending[0]
	if summary.ResultNodeID != "n_script" || summary.OutputBinding == nil || *summary.OutputBinding != *binding {
		t.Fatalf("frozen pending target = %+v", summary)
	}
	view.Pending[0].OutputBinding.NodeID = "n_reparented"
	if summary.OutputBinding.NodeID != "n_script" {
		t.Fatalf("summary binding was not copied: %+v", summary.OutputBinding)
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if containsAny(string(encoded), "private payload", "rawPayload") {
		t.Fatalf("workspace snapshot contains pending payload: %s", encoded)
	}
}

func TestWorkspaceSnapshotEmptyAndRetryableOnRevisionRace(t *testing.T) {
	service, workflowService, revision, layout := newWorkspaceFixture(t, false)
	manager := runservice.New(runservice.Options{Projects: service, RunStores: persistence.NewRunStoreProvider(layout)})
	handler := NewWorkspaceService(workflowService, manager, revision)
	empty, err := handler.GetWorkspaceSnapshot()
	if err != nil || empty.Snapshot.Project != nil || empty.Snapshot.Nodes == nil || empty.Snapshot.Pending == nil || empty.Snapshot.Runs == nil || empty.Snapshot.Chains == nil || empty.Snapshot.LockedNodeIDs == nil {
		t.Fatalf("empty snapshot = %+v, err=%v", empty, err)
	}
	projectValue, err := service.New(context.Background(), "工作台")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workflowService.AddNode(context.Background(), workflow.AddNodeInput{
		Type: valueobject.NodeTypeTopic, Title: "主题", Position: entity.Position{},
	}); err != nil {
		t.Fatal(err)
	}
	revision.mu.Lock()
	revision.unstable = true
	revision.projectID = projectValue.ID
	revision.mu.Unlock()
	if _, err := handler.GetWorkspaceSnapshot(); !errors.Is(err, ErrWorkspaceChangedDuringSnapshot) {
		t.Fatalf("snapshot race error = %v", err)
	}
}

func newWorkspaceFixture(t *testing.T, active bool) (*project.Service, *workflow.Service, *testWorkspaceRevision, valueobject.Layout) {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	layout := valueobject.Layout{
		Root: root, DataDir: dataDir, LogsDir: filepath.Join(root, "logs"),
		WebView2Dir: filepath.Join(root, "webview2"), LogFile: filepath.Join(root, "logs", "app.log"),
		AgentDir: filepath.Join(dataDir, "agent"), ConfigFile: filepath.Join(dataDir, "config.json"),
		ProjectsDir: filepath.Join(dataDir, "projects"), BackupDir: filepath.Join(dataDir, "backup", "m1"),
	}
	revision := &testWorkspaceRevision{revision: 1}
	service := project.New(project.Options{
		Layout: layout, Config: persistence.NewConfigStore(layout.ConfigFile),
		Projects: persistence.NewProjectStore(layout), OnChange: revision.projectChanged,
	})
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if active {
		if _, err := service.New(context.Background(), "fixture"); err != nil {
			t.Fatal(err)
		}
	}
	workflowService := workflow.New(workflow.Options{
		Project:   service,
		Pending:   persistence.NewPendingStoreProvider(layout),
		Artifacts: persistence.NewArtifactStoreProvider(layout),
		OnChange:  revision.workflowChanged,
	})
	return service, workflowService, revision, layout
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
