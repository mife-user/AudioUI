package handler

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/persistence"
	"audio-ui/internal/service/project"
)

func newProjectHandler(t *testing.T) (*ProjectService, valueobject.Layout) {
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
	service := project.New(project.Options{
		Layout:   layout,
		Config:   persistence.NewConfigStore(layout.ConfigFile),
		Projects: persistence.NewProjectStore(layout),
	})
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	return NewProjectService(service), layout
}

func TestProjectHandlerEmptyInputs(t *testing.T) {
	h, _ := newProjectHandler(t)
	if _, err := h.NewProject("  "); err == nil || !strings.Contains(err.Error(), "工程名称不能为空") {
		t.Fatalf("NewProject error = %v", err)
	}
	if _, err := h.OpenProject(""); err == nil || !strings.Contains(err.Error(), "工程 ID 不能为空") {
		t.Fatalf("OpenProject error = %v", err)
	}
	if _, err := h.GetActiveProject(); err == nil || !strings.Contains(err.Error(), "NO_ACTIVE_PROJECT") {
		t.Fatalf("GetActiveProject error = %v", err)
	}
}

func TestProjectHandlerNewListOpenConfig(t *testing.T) {
	h, _ := newProjectHandler(t)
	projectValue, err := h.NewProject(" 示例 ")
	if err != nil {
		t.Fatal(err)
	}
	if projectValue.Name != "示例" {
		t.Fatalf("name = %q", projectValue.Name)
	}
	list, err := h.ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != projectValue.ID {
		t.Fatalf("list = %+v", list)
	}
	active, err := h.GetActiveProject()
	if err != nil {
		t.Fatal(err)
	}
	if active.ID != projectValue.ID {
		t.Fatalf("active = %+v", active)
	}
	config, err := h.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.LastProjectID != projectValue.ID || config.ContextTokenBudget != valueobject.DefaultContextTokenBudget {
		t.Fatalf("config = %+v", config)
	}
	if _, err := h.OpenProject(projectValue.ID); err != nil {
		t.Fatal(err)
	}
}

func TestProjectHandlerSurfacesInvalidID(t *testing.T) {
	h, _ := newProjectHandler(t)
	_, err := h.OpenProject("../escape")
	if !errors.Is(err, project.ErrProjectIDInvalid) {
		t.Fatalf("error = %v, want ErrProjectIDInvalid", err)
	}
}
