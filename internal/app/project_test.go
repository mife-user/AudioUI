package app

import (
	"context"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/handler"
	"audio-ui/internal/infra/persistence"

	"go.uber.org/zap"
)

func TestNewProjectSetupMigratesM1Data(t *testing.T) {
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
	sessions := valueobject.SessionsFile{
		Version: valueobject.SessionFileVersion, ActiveChainID: valueobject.DefaultChainID,
		Chains: map[string]valueobject.SessionRecord{
			valueobject.DefaultChainID: {
				Adapter: "claude", DisplayID: "active", Status: valueobject.SessionIdle,
				UpdatedAt: "2026-10-31T10:00:00Z",
				Transcript: []valueobject.TranscriptItem{{
					ID: "m1", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "旧对话",
				}},
			},
		},
	}
	if err := persistence.NewSessionStore(layout.SessionsFile).Save(context.Background(), sessions); err != nil {
		t.Fatal(err)
	}

	setup := newProjectSetup(layout, zap.NewNop())
	list, err := setup.handler.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "M1 迁移" {
		t.Fatalf("list = %+v", list)
	}
	active, err := setup.handler.GetActiveProject()
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != list[0].ID || len(active.Nodes) != 1 {
		t.Fatalf("active = %+v", active)
	}
	if _, err := persistence.NewRunStore(filepath.Join(layout.ProjectsDir, active.ID)).List(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNewWorkflowSetupWiresCommands(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	layout := valueobject.Layout{
		Root: root, DataDir: dataDir, LogsDir: filepath.Join(root, "logs"),
		WebView2Dir: filepath.Join(root, "webview2"), LogFile: filepath.Join(root, "logs", "app.log"),
		AgentDir: filepath.Join(dataDir, "agent"), WorkspaceDir: filepath.Join(dataDir, "workspace", "chain_default"),
		SessionsFile: filepath.Join(dataDir, "sessions.json"), ConfigFile: filepath.Join(dataDir, "config.json"),
		ProjectsDir: filepath.Join(dataDir, "projects"), BackupDir: filepath.Join(dataDir, "backup", "m1"),
	}
	projectSetup := newProjectSetup(layout, zap.NewNop())
	if _, err := projectSetup.handler.NewProject("工程"); err != nil {
		t.Fatal(err)
	}
	workflowSetup := newWorkflowSetup(layout, projectSetup.service)
	projectValue, err := workflowSetup.handler.AddNode(handler.AddNodeRequest{
		Type: "topic", Title: "选题", Position: entity.Position{X: 1, Y: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projectValue.Nodes) != 1 || projectValue.Nodes[0].Type != valueobject.NodeTypeTopic {
		t.Fatalf("project = %+v", projectValue)
	}
}
