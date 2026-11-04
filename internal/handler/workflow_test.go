package handler

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/persistence"
	"audio-ui/internal/service/project"
	"audio-ui/internal/service/workflow"
)

type handlerCandidates struct{}

func (handlerCandidates) Count(ctx context.Context, projectID string, node entity.Node) (int, error) {
	return 3, nil
}

type handlerContextHasher struct{}

func (handlerContextHasher) InputHashes(context.Context, entity.Project, string) (valueobject.ContextHashes, error) {
	return valueobject.ContextHashes{ContextHash: "ctx", AncestorHash: "ancestor", ParamsHash: "params"}, nil
}

func newWorkflowHandler(t *testing.T) *WorkflowService {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	layout := valueobject.Layout{
		Root:         root,
		DataDir:      dataDir,
		LogsDir:      filepath.Join(root, "logs"),
		WebView2Dir:  filepath.Join(root, "webview2"),
		LogFile:      filepath.Join(root, "logs", "app.log"),
		RootSource:   "exe",
		Portable:     true,
		AgentDir:     filepath.Join(dataDir, "agent"),
		WorkspaceDir: filepath.Join(dataDir, "workspace", "chain_default"),
		SessionsFile: filepath.Join(dataDir, "sessions.json"),
		AuditFile:    filepath.Join(root, "logs", "audit.log"),
		ConfigFile:   filepath.Join(dataDir, "config.json"),
		ProjectsDir:  filepath.Join(dataDir, "projects"),
		BackupDir:    filepath.Join(dataDir, "backup", "m1"),
	}
	projectService := project.New(project.Options{
		Layout: layout, Config: persistence.NewConfigStore(layout.ConfigFile),
		Projects: persistence.NewProjectStore(layout),
	})
	if err := projectService.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := projectService.New(context.Background(), "工程"); err != nil {
		t.Fatal(err)
	}
	artifactStores := persistence.NewArtifactStoreProvider(layout)
	workflowService := workflow.New(workflow.Options{
		Project: projectService, Candidates: handlerCandidates{},
		Pending: persistence.NewPendingStoreProvider(layout), Artifacts: artifactStores,
		ContextHasher: handlerContextHasher{},
	})
	return NewWorkflowService(workflowService)
}

func activeProjectID(t *testing.T, h *WorkflowService) string {
	t.Helper()
	view, err := h.service.WorkspaceView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return view.Project.ID
}

func TestWorkflowHandlerAddMoveUpdateSelect(t *testing.T) {
	h := newWorkflowHandler(t)
	projectID := activeProjectID(t, h)
	topicReceipt, err := h.AddNode(AddNodeRequest{
		ProjectID: projectID, Type: "topic", Title: " 选题 ", Position: entity.Position{X: 1, Y: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if topicReceipt.ProjectID == "" || topicReceipt.ProjectRevision == 0 || topicReceipt.NodeID == "" {
		t.Fatalf("topic receipt = %+v", topicReceipt)
	}

	promptReceipt, err := h.AddNode(AddNodeRequest{
		ProjectID: projectID, Type: "prompt", Title: "提示词", Position: entity.Position{X: 3, Y: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	promptID := promptReceipt.NodeID
	if promptReceipt.ProjectID != topicReceipt.ProjectID || promptReceipt.ProjectRevision <= topicReceipt.ProjectRevision {
		t.Fatalf("prompt receipt = %+v, topic receipt = %+v", promptReceipt, topicReceipt)
	}

	selected, err := h.SelectNode(promptID, topicReceipt.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if selected.NodeID != promptID || selected.ProjectRevision != promptReceipt.ProjectRevision {
		t.Fatalf("select receipt = %+v", selected)
	}

	moved, err := h.MoveNode(promptID, 8, 9, topicReceipt.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	view, err := h.service.WorkspaceView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if moved.ProjectRevision <= selected.ProjectRevision || view.Project.ActiveNodeID != promptID {
		t.Fatalf("move receipt = %+v, project = %+v", moved, view.Project)
	}
	var prompt entity.Node
	for _, node := range view.Project.Nodes {
		if node.ID == promptID {
			prompt = node
		}
	}
	if prompt.Position != (entity.Position{X: 8, Y: 9}) {
		t.Fatalf("moved prompt = %+v", prompt)
	}

	updated, err := h.UpdateNodeParams(promptID, "新标题", entity.NodeParams{
		Instruction: "写脚本", OutputKind: valueobject.OutputKindScript,
	}, topicReceipt.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.NodeID != promptID || updated.ProjectRevision <= moved.ProjectRevision {
		t.Fatalf("update receipt = %+v", updated)
	}
	view, err = h.service.WorkspaceView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range view.Project.Nodes {
		if node.ID == promptID && (node.Title != "新标题" || node.Params.Instruction != "写脚本") {
			t.Fatalf("updated prompt = %+v", node)
		}
	}

	candidate := 0
	connected, err := h.Connect(topicReceipt.NodeID, promptID, &candidate, false, topicReceipt.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if connected.NodeID != promptID || connected.ProjectID != topicReceipt.ProjectID {
		t.Fatalf("connect receipt = %+v", connected)
	}
}

func TestWorkflowHandlerPendingQueriesApplyAndDiscard(t *testing.T) {
	h := newWorkflowHandler(t)
	projectID := activeProjectID(t, h)
	topicReceipt, err := h.AddNode(AddNodeRequest{ProjectID: projectID, Type: "topic", Title: "选题"})
	if err != nil {
		t.Fatal(err)
	}
	topicID := topicReceipt.NodeID
	candidate := 0
	firstReceipt, err := h.AddNode(AddNodeRequest{
		ProjectID: projectID, Type: "prompt", Title: "脚本 A", Parent: topicID, ParentCandidate: &candidate,
		Params: entity.NodeParams{OutputKind: valueobject.OutputKindScript},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstNodeID := firstReceipt.NodeID
	secondReceipt, err := h.AddNode(AddNodeRequest{
		ProjectID: projectID, Type: "prompt", Title: "脚本 B", Parent: topicID, ParentCandidate: &candidate,
		Params: entity.NodeParams{OutputKind: valueobject.OutputKindScript},
	})
	if err != nil {
		t.Fatal(err)
	}
	secondNodeID := secondReceipt.NodeID
	payload := []byte(`{"title":"脚本","scenes":[{"narration":"旁白","duration":0}]}`)
	first, err := h.service.CreatePending(context.Background(), workflow.CreatePendingInput{
		RunID: "r_first", ChainID: "c_" + firstNodeID, ChainPath: []string{topicID, firstNodeID},
		NodeID: firstNodeID, Kind: valueobject.OutputKindScript,
		ContextHash: "ctx", AncestorHash: "ancestor", ParamsHash: "params", Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := h.service.CreatePending(context.Background(), workflow.CreatePendingInput{
		RunID: "r_second", ChainID: "c_" + secondNodeID, ChainPath: []string{topicID, secondNodeID},
		NodeID: secondNodeID, Kind: valueobject.OutputKindScript,
		ContextHash: "ctx", AncestorHash: "ancestor", ParamsHash: "params", Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := h.ListPending()
	if err != nil || len(list) != 2 {
		t.Fatalf("pending list = %+v err=%v", list, err)
	}
	detail, err := h.GetPending(second.ID)
	if err != nil || string(detail.RawPayload) != string(payload) {
		t.Fatalf("pending detail=%+v err=%v", detail, err)
	}
	applied, err := h.ApplyPending(first.ID, false)
	if err != nil || !applied.Nodes[1].Locked {
		t.Fatalf("apply result=%+v err=%v", applied, err)
	}
	if _, err := h.DiscardPending(second.ID); err != nil {
		t.Fatal(err)
	}
	list, err = h.ListPending()
	if err != nil || list[0].Status != valueobject.PendingApplied || list[1].Status != valueobject.PendingDiscarded {
		t.Fatalf("pending statuses = %+v err=%v", list, err)
	}
}

func TestWorkflowHandlerValidation(t *testing.T) {
	h := newWorkflowHandler(t)
	if _, err := h.AddNode(AddNodeRequest{ProjectID: activeProjectID(t, h), Type: "render"}); !errors.Is(err, workflow.ErrUnknownNodeType) {
		t.Fatalf("error = %v, want UNKNOWN_NODE_TYPE", err)
	}
	if _, err := h.Connect("", "n_child", nil, false, ""); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("Connect error = %v", err)
	}
	if _, err := h.MoveNode("", 1, 1, ""); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("MoveNode error = %v", err)
	}
	if _, err := h.SelectNode("", ""); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("SelectNode error = %v", err)
	}
}
