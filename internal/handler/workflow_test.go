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

type handlerContextHasher struct{}

func (handlerContextHasher) InputHashes(context.Context, entity.Project, string) (valueobject.ContextHashes, error) {
	return valueobject.ContextHashes{ContextHash: "ctx", AncestorHash: "ancestor", ParamsHash: "params"}, nil
}

func newWorkflowHandler(t *testing.T) *WorkflowService {
	handler, _ := newWorkflowHandlerWithProjectService(t)
	return handler
}

func newWorkflowHandlerWithProjectService(t *testing.T) (*WorkflowService, *project.Service) {
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
		Project:         projectService,
		CandidateReader: workflow.NewArtifactCandidateCounter(artifactStores),
		Pending:         persistence.NewPendingStoreProvider(layout), Artifacts: artifactStores,
		ContextHasher: handlerContextHasher{},
	})
	return NewWorkflowService(workflowService), projectService
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
	view, err := h.service.WorkspaceView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range view.Project.Nodes {
		if node.ID == topicReceipt.NodeID && node.Size != entity.DefaultNodeSize(valueobject.NodeTypeTopic) {
			t.Fatalf("topic size = %+v", node.Size)
		}
		if node.ID == promptID && node.Size != entity.DefaultNodeSize(valueobject.NodeTypePrompt) {
			t.Fatalf("prompt size = %+v", node.Size)
		}
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
	view, err = h.service.WorkspaceView(context.Background())
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

	layout, err := h.SetNodeLayout(promptID, 10, 11, 340, 260, topicReceipt.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	view, err = h.service.WorkspaceView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range view.Project.Nodes {
		if node.ID == promptID && (node.Position != (entity.Position{X: 10, Y: 11}) || node.Size != (entity.NodeSize{Width: 340, Height: 260})) {
			t.Fatalf("layout result = %+v", node)
		}
	}
	if layout.NodeID != promptID || layout.ProjectRevision <= moved.ProjectRevision {
		t.Fatalf("layout receipt = %+v", layout)
	}

	collapsed, err := h.SetNodeCollapsed(promptID, true, topicReceipt.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	view, err = h.service.WorkspaceView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range view.Project.Nodes {
		if node.ID == promptID && !node.Collapsed {
			t.Fatalf("collapsed prompt = %+v", node)
		}
	}
	if collapsed.NodeID != promptID || collapsed.ProjectRevision <= layout.ProjectRevision {
		t.Fatalf("collapsed receipt = %+v", collapsed)
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

	if _, err := h.Connect(promptID, promptID, topicReceipt.ProjectID); !errors.Is(err, workflow.ErrSelfConnect) {
		t.Fatalf("self connection error = %v, want SELF_CONNECT", err)
	}
	if _, err := h.Connect(topicReceipt.NodeID, promptID, topicReceipt.ProjectID); err != nil {
		t.Fatalf("arbitrary node connection = %v", err)
	}
}

func TestWorkflowHandlerPendingQueriesApplyAndDiscard(t *testing.T) {
	h := newWorkflowHandler(t)
	projectID := activeProjectID(t, h)
	firstReceipt, err := h.AddNode(AddNodeRequest{ProjectID: projectID, Type: "topic", Title: "选题 A"})
	if err != nil {
		t.Fatal(err)
	}
	secondReceipt, err := h.AddNode(AddNodeRequest{ProjectID: projectID, Type: "topic", Title: "选题 B"})
	if err != nil {
		t.Fatal(err)
	}
	firstTopicID, secondTopicID := firstReceipt.NodeID, secondReceipt.NodeID
	candidates, err := h.GetTopicCandidates(firstTopicID, projectID)
	if err != nil || len(candidates) != 0 {
		t.Fatalf("unapplied candidates = %+v err=%v", candidates, err)
	}
	if _, err := h.GetTopicCandidates(firstTopicID, "p_other"); !errors.Is(err, workflow.ErrProjectChanged) {
		t.Fatalf("candidate project scope error = %v", err)
	}

	payload := []byte(`{"topics":[{"title":"甲","angle":"角度甲","hook":"开头甲"},{"title":"乙","angle":"角度乙","hook":"开头乙"},{"title":"丙","angle":"角度丙","hook":"开头丙"}]}`)
	first, err := h.service.CreatePending(context.Background(), workflow.CreatePendingInput{
		RunID: "r_first", TreeID: firstTopicID, ChainID: "c_" + firstTopicID, ChainPath: []string{firstTopicID},
		NodeID: firstTopicID, Kind: valueobject.OutputKindTopics,
		ContextHash: "ctx", AncestorHash: "ancestor", ParamsHash: "params", Payload: payload,
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidPayload := []byte(`{"topics":[{"title":"未达标","angle":"角度","hook":"开头"}]}`)
	second, err := h.service.CreatePending(context.Background(), workflow.CreatePendingInput{
		RunID: "r_second", TreeID: secondTopicID, ChainID: "c_" + secondTopicID, ChainPath: []string{secondTopicID},
		NodeID: secondTopicID, Kind: valueobject.OutputKindTopics,
		ContextHash: "ctx", AncestorHash: "ancestor", ParamsHash: "params", Payload: invalidPayload,
	})
	if err != nil {
		t.Fatal(err)
	}
	list, err := h.ListPending(activeProjectID(t, h))
	if err != nil || len(list) != 2 {
		t.Fatalf("pending list = %+v err=%v", list, err)
	}
	detail, err := h.GetPending(second.ID, activeProjectID(t, h))
	if err != nil || string(detail.RawPayload) != string(invalidPayload) {
		t.Fatalf("pending detail=%+v err=%v", detail, err)
	}
	if _, err := h.ApplyPending(first.ID, false, "p_other"); !errors.Is(err, workflow.ErrProjectChanged) {
		t.Fatalf("pending apply project scope error = %v", err)
	}
	applied, err := h.ApplyPending(first.ID, false, activeProjectID(t, h))
	if err != nil {
		t.Fatal(err)
	}
	var appliedTopic *entity.Node
	for i := range applied.Nodes {
		if applied.Nodes[i].ID == firstTopicID {
			appliedTopic = &applied.Nodes[i]
		}
	}
	if appliedTopic == nil || !appliedTopic.Locked || len(appliedTopic.Rounds) != 1 {
		t.Fatalf("apply result=%+v", applied)
	}
	candidates, err = h.GetTopicCandidates(firstTopicID, activeProjectID(t, h))
	if err != nil || len(candidates) != 3 {
		t.Fatalf("applied candidates = %+v err=%v", candidates, err)
	}
	if _, err := h.DiscardPending(second.ID, activeProjectID(t, h)); err != nil {
		t.Fatal(err)
	}
	list, err = h.ListPending(activeProjectID(t, h))
	if err != nil || list[0].Status != valueobject.PendingApplied || list[1].Status != valueobject.PendingDiscarded {
		t.Fatalf("pending statuses = %+v err=%v", list, err)
	}
}

func TestWorkflowHandlerReconnectTargetPersistsAndChecksScope(t *testing.T) {
	h, projects := newWorkflowHandlerWithProjectService(t)
	projectID := activeProjectID(t, h)
	topic, err := h.AddNode(AddNodeRequest{ProjectID: projectID, Type: "topic", Title: "选题"})
	if err != nil {
		t.Fatal(err)
	}
	addPrompt := func(title string) string {
		t.Helper()
		receipt, err := h.AddNode(AddNodeRequest{ProjectID: projectID, Type: "prompt", Title: title, Parent: topic.NodeID})
		if err != nil {
			t.Fatal(err)
		}
		return receipt.NodeID
	}
	sourceID := addPrompt("来源")
	oldTargetID := addPrompt("旧目标")
	newTargetID := addPrompt("新目标")
	connected, err := h.Connect(sourceID, oldTargetID, projectID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := h.service.WorkspaceView(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var expectedEdge entity.Edge
	for _, edge := range before.Project.Edges {
		if edge.SourceNodeID == sourceID && edge.TargetNodeID == oldTargetID {
			expectedEdge = edge
			break
		}
	}
	if expectedEdge.ID == "" {
		t.Fatal("connected edge not found")
	}

	receipt, err := h.ReconnectTarget(ReconnectTargetRequest{
		ProjectID: projectID, ExpectedEdge: expectedEdge, TargetNodeID: newTargetID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ProjectID != projectID || receipt.ProjectRevision != connected.ProjectRevision+1 || receipt.NodeID != newTargetID {
		t.Fatalf("reconnect receipt = %+v", receipt)
	}
	reopened, err := projects.Open(context.Background(), projectID)
	if err != nil {
		t.Fatal(err)
	}
	var persisted entity.Edge
	for _, edge := range reopened.Edges {
		if edge.ID == expectedEdge.ID {
			persisted = edge
			break
		}
	}
	if persisted.ID != expectedEdge.ID || persisted.SourceNodeID != sourceID || persisted.TargetNodeID != newTargetID || persisted.Kind != expectedEdge.Kind || persisted.ScriptVersion != expectedEdge.ScriptVersion {
		t.Fatalf("persisted edge = %+v", persisted)
	}

	if _, err := h.ReconnectTarget(ReconnectTargetRequest{
		ProjectID: projectID, ExpectedEdge: expectedEdge, TargetNodeID: newTargetID,
	}); !errors.Is(err, workflow.ErrEdgeConflict) {
		t.Fatalf("stale edge error = %v, want EDGE_CONFLICT", err)
	}
	currentEdge := persisted
	if _, err := h.ReconnectTarget(ReconnectTargetRequest{
		ProjectID: "p_other", ExpectedEdge: currentEdge, TargetNodeID: oldTargetID,
	}); !errors.Is(err, workflow.ErrProjectChanged) {
		t.Fatalf("project scope error = %v, want PROJECT_CHANGED", err)
	}
	current, err := projects.Current(context.Background())
	if err != nil || current.Revision != reopened.Revision {
		t.Fatalf("failed stale requests changed persisted revision: revision=%d err=%v", current.Revision, err)
	}
}

func TestWorkflowHandlerReconnectTargetRequiresCompleteIdentity(t *testing.T) {
	h := newWorkflowHandler(t)
	if _, err := h.ReconnectTarget(ReconnectTargetRequest{}); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("reconnect validation error = %v", err)
	}
}

func TestWorkflowHandlerValidation(t *testing.T) {
	h := newWorkflowHandler(t)
	if _, err := h.AddNode(AddNodeRequest{ProjectID: activeProjectID(t, h), Type: "render"}); !errors.Is(err, workflow.ErrUnknownNodeType) {
		t.Fatalf("error = %v, want UNKNOWN_NODE_TYPE", err)
	}
	if _, err := h.Connect("", "n_child", ""); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("Connect error = %v", err)
	}
	if _, err := h.MoveNode("", 1, 1, ""); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("MoveNode error = %v", err)
	}
	if _, err := h.SelectNode("", ""); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("SelectNode error = %v", err)
	}
}
