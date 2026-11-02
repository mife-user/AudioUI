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
	workflowService := workflow.New(workflow.Options{
		Project: projectService, Candidates: handlerCandidates{},
	})
	return NewWorkflowService(workflowService)
}

func TestWorkflowHandlerAddMoveUpdateSelect(t *testing.T) {
	h := newWorkflowHandler(t)
	projectValue, err := h.AddNode(AddNodeRequest{
		Type: "topic", Title: " 选题 ", Position: entity.Position{X: 1, Y: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projectValue.Nodes) != 1 || projectValue.Nodes[0].Title != "选题" {
		t.Fatalf("project = %+v", projectValue)
	}
	topicID := projectValue.Nodes[0].ID

	projectValue, err = h.AddNode(AddNodeRequest{
		Type: "prompt", Title: "提示词", Position: entity.Position{X: 3, Y: 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	promptID := projectValue.Nodes[len(projectValue.Nodes)-1].ID

	projectValue, err = h.SelectNode(promptID)
	if err != nil {
		t.Fatal(err)
	}
	if projectValue.ActiveNodeID != promptID || projectValue.Revision != 3 {
		t.Fatalf("select result = %+v", projectValue)
	}

	projectValue, err = h.MoveNode(promptID, 8, 9)
	if err != nil {
		t.Fatal(err)
	}
	last := projectValue.Nodes[len(projectValue.Nodes)-1]
	if last.Position != (entity.Position{X: 8, Y: 9}) || projectValue.Revision != 4 {
		t.Fatalf("move result = %+v", projectValue)
	}

	projectValue, err = h.UpdateNodeParams(promptID, "新标题", entity.NodeParams{
		Instruction: "写脚本", OutputKind: valueobject.OutputKindScript,
	})
	if err != nil {
		t.Fatal(err)
	}
	last = projectValue.Nodes[len(projectValue.Nodes)-1]
	if last.Title != "新标题" || last.Params.Instruction != "写脚本" {
		t.Fatalf("update result = %+v", last)
	}

	candidate := 0
	if _, err := h.Connect(topicID, promptID, &candidate, false); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowHandlerValidation(t *testing.T) {
	h := newWorkflowHandler(t)
	if _, err := h.AddNode(AddNodeRequest{Type: "render"}); !errors.Is(err, workflow.ErrUnknownNodeType) {
		t.Fatalf("error = %v, want UNKNOWN_NODE_TYPE", err)
	}
	if _, err := h.Connect("", "n_child", nil, false); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("Connect error = %v", err)
	}
	if _, err := h.MoveNode("", 1, 1); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("MoveNode error = %v", err)
	}
	if _, err := h.SelectNode(""); err == nil || !strings.Contains(err.Error(), "不能为空") {
		t.Fatalf("SelectNode error = %v", err)
	}
}
