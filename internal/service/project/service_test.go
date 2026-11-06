package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/persistence"
)

func testLayout(root string) valueobject.Layout {
	dataDir := filepath.Join(root, "data")
	return valueobject.Layout{
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
}

func newTestService(layout valueobject.Layout) *Service {
	return New(Options{
		Layout:   layout,
		Config:   persistence.NewConfigStore(layout.ConfigFile),
		Projects: persistence.NewProjectStore(layout),
		Migrator: persistence.NewM1MigratorWithNow(layout, func() time.Time {
			return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
		}),
	})
}

func TestServiceStartWithoutProjects(t *testing.T) {
	layout := testLayout(t.TempDir())
	service := newTestService(layout)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Current(context.Background()); !errors.Is(err, ErrNoActiveProject) {
		t.Fatalf("Current error = %v, want ErrNoActiveProject", err)
	}
	config, err := service.Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.ContextTokenBudget != valueobject.DefaultContextTokenBudget || config.LastProjectID != "" {
		t.Fatalf("config = %+v", config)
	}
}

func TestServiceUnsupportedLastProjectDoesNotBlockNewOrOpen(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := persistence.NewProjectStore(layout)
	valid := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_valid", Name: "有效工程", Revision: 1, Nodes: []entity.Node{},
	}
	if err := store.Create(context.Background(), valid); err != nil {
		t.Fatal(err)
	}
	old := entity.Project{
		Version: 1, ID: "p_old", Name: "旧工程", Revision: 1, Nodes: []entity.Node{},
	}
	oldDir := filepath.Join(layout.ProjectsDir, old.ID)
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "project.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := persistence.NewConfigStore(layout.ConfigFile).Save(context.Background(), valueobject.AppConfig{
		LastProjectID: old.ID, ContextTokenBudget: 321,
	}); err != nil {
		t.Fatal(err)
	}

	service := newTestService(layout)
	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if _, err := service.Current(context.Background()); !errors.Is(err, ErrNoActiveProject) {
		t.Fatalf("Current() error = %v, want NO_ACTIVE_PROJECT", err)
	}
	list, err := service.List(context.Background())
	if err != nil || len(list) != 2 {
		t.Fatalf("List() = %+v, err = %v", list, err)
	}
	if _, err := service.Open(context.Background(), old.ID); !errors.Is(err, persistence.ErrUnsupportedVersion) {
		t.Fatalf("Open(old) error = %v, want UNSUPPORTED_VERSION", err)
	}
	created, err := service.New(context.Background(), "新工程")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if created.Name != "新工程" {
		t.Fatalf("created = %+v", created)
	}
	if _, err := service.Open(context.Background(), valid.ID); err != nil {
		t.Fatalf("Open(valid) error = %v", err)
	}
}

func TestServiceNewPersistsAndRestoresLastProject(t *testing.T) {
	layout := testLayout(t.TempDir())
	service := newTestService(layout)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	created, err := service.New(context.Background(), "我的工程")
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "我的工程" || created.Revision != 1 || len(created.Nodes) != 0 {
		t.Fatalf("created = %+v", created)
	}
	list, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != created.ID {
		t.Fatalf("list = %+v", list)
	}
	config, err := service.Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.LastProjectID != created.ID {
		t.Fatalf("lastProjectId = %q", config.LastProjectID)
	}

	restarted := newTestService(layout)
	if err := restarted.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, err := restarted.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != created.ID || current.Name != "我的工程" {
		t.Fatalf("current = %+v", current)
	}
}

func TestServiceOpenSwitchesAndPersists(t *testing.T) {
	layout := testLayout(t.TempDir())
	service := newTestService(layout)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := service.New(context.Background(), "甲")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.New(context.Background(), "乙")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Open(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	current, err := service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != first.ID {
		t.Fatalf("current = %+v", current)
	}

	restarted := newTestService(layout)
	if err := restarted.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, err = restarted.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.ID != first.ID {
		t.Fatalf("restored = %+v, want %q (second=%q)", current, first.ID, second.ID)
	}
}

func TestProjectChangeCallbackCarriesCommittedIdentityAndRevision(t *testing.T) {
	layout := testLayout(t.TempDir())
	var changes []struct {
		id       string
		revision int64
	}
	service := New(Options{
		Layout: layout, Config: persistence.NewConfigStore(layout.ConfigFile),
		Projects: persistence.NewProjectStore(layout),
		OnChange: func(id string, revision int64) {
			changes = append(changes, struct {
				id       string
				revision int64
			}{id: id, revision: revision})
		},
	})
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := service.New(context.Background(), "甲")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.New(context.Background(), "乙")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Open(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Open(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 3 || changes[0].id != first.ID || changes[1].id != second.ID || changes[2].id != first.ID {
		t.Fatalf("project callbacks = %+v", changes)
	}
	for _, change := range changes {
		if change.revision != 1 {
			t.Fatalf("project callback revision = %+v, want initial graph revision 1", change)
		}
	}
}

func TestProjectSwitchIsRejectedWhileRunIsActive(t *testing.T) {
	service := newTestService(testLayout(t.TempDir()))
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	first, err := service.New(context.Background(), "甲")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.New(context.Background(), "乙")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Open(context.Background(), first.ID); err != nil {
		t.Fatal(err)
	}
	service.SetRunActiveCheck(func() bool { return true })
	if _, err := service.Open(context.Background(), second.ID); !errors.Is(err, ErrRunInProgress) {
		t.Fatalf("Open error = %v, want ErrRunInProgress", err)
	}
	if _, err := service.New(context.Background(), "丙"); !errors.Is(err, ErrRunInProgress) {
		t.Fatalf("New error = %v, want ErrRunInProgress", err)
	}
	if _, err := service.Open(context.Background(), first.ID); err != nil {
		t.Fatalf("reopening the active project should be safe: %v", err)
	}
	current, err := service.Current(context.Background())
	if err != nil || current.ID != first.ID {
		t.Fatalf("current project = %q, err=%v", current.ID, err)
	}
}

func TestServiceRejectsInvalidNames(t *testing.T) {
	service := newTestService(testLayout(t.TempDir()))
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "   ", strings.Repeat("长", valueobject.MaxProjectNameLength+1)} {
		if _, err := service.New(context.Background(), name); !errors.Is(err, ErrProjectNameInvalid) {
			t.Fatalf("New(%q) error = %v, want ErrProjectNameInvalid", name, err)
		}
	}
}

func TestServiceRejectsInvalidProjectID(t *testing.T) {
	service := newTestService(testLayout(t.TempDir()))
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"", "..", "p/one"} {
		if _, err := service.Open(context.Background(), id); !errors.Is(err, ErrProjectIDInvalid) {
			t.Fatalf("Open(%q) error = %v, want ErrProjectIDInvalid", id, err)
		}
	}
}

func TestServiceConfigCorruptionIsStickyAndPreserved(t *testing.T) {
	layout := testLayout(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(layout.ConfigFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.ConfigFile, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := newTestService(layout)
	if err := service.Start(context.Background()); err == nil {
		t.Fatal("Start error = nil, want corrupt config error")
	}
	if _, err := service.New(context.Background(), "工程"); err == nil {
		t.Fatal("New error = nil, want config error")
	}
	data, err := os.ReadFile(layout.ConfigFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{" {
		t.Fatalf("corrupt config was rewritten: %q", data)
	}
}

func TestServiceStartRunsM1Migration(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeSessionsFile(t, layout)
	service := newTestService(layout)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, err := service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Name != "M1 迁移" || len(current.Nodes) != 1 {
		t.Fatalf("current = %+v", current)
	}
	config, err := service.Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.LastProjectID != current.ID {
		t.Fatalf("lastProjectId = %q, want %q", config.LastProjectID, current.ID)
	}
}

func TestServiceStartReportsMigrationFailure(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeSessionsFile(t, layout)
	if err := os.MkdirAll(filepath.Dir(layout.BackupDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.BackupDir, []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := newTestService(layout)
	err := service.Start(context.Background())
	if !errors.Is(err, port.ErrMigrationFailed) {
		t.Fatalf("Start error = %v, want MIGRATION_FAILED", err)
	}
}

func TestServiceMissingLastProjectIsExplicitAndRecoverable(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := persistence.NewConfigStore(layout.ConfigFile)
	if err := store.Save(context.Background(), valueobject.AppConfig{LastProjectID: "p_missing", ContextTokenBudget: 1000}); err != nil {
		t.Fatal(err)
	}
	service := newTestService(layout)
	if err := service.Start(context.Background()); err == nil {
		t.Fatal("Start error = nil, want missing project error")
	}
	if _, err := service.Current(context.Background()); !errors.Is(err, ErrNoActiveProject) {
		t.Fatalf("Current error = %v, want ErrNoActiveProject", err)
	}
	created, err := service.New(context.Background(), "恢复工程")
	if err != nil {
		t.Fatal(err)
	}
	config, err := service.Config(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if config.LastProjectID != created.ID {
		t.Fatalf("lastProjectId = %q", config.LastProjectID)
	}
}

func TestServiceSaveActiveProject(t *testing.T) {
	layout := testLayout(t.TempDir())
	service := newTestService(layout)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	created, err := service.New(context.Background(), "工程")
	if err != nil {
		t.Fatal(err)
	}
	wrong := created.Clone()
	wrong.ID = "p_other"
	if _, err := service.Save(context.Background(), wrong); !errors.Is(err, ErrActiveProjectChanged) {
		t.Fatalf("wrong id error = %v, want ErrActiveProjectChanged", err)
	}
	created.Name = "改名"
	created.Revision = 5
	saved, err := service.Save(context.Background(), created)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name != "改名" || saved.Revision != 5 {
		t.Fatalf("saved = %+v", saved)
	}
	restarted := newTestService(layout)
	if err := restarted.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, err := restarted.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if current.Name != "改名" || current.Revision != 5 {
		t.Fatalf("current = %+v", current)
	}
}

func TestServiceSaveWithoutActiveProject(t *testing.T) {
	service := newTestService(testLayout(t.TempDir()))
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Save(context.Background(), entity.Project{ID: "p_test", Version: 1}); !errors.Is(err, ErrNoActiveProject) {
		t.Fatalf("error = %v, want ErrNoActiveProject", err)
	}
}

func TestServiceCurrentReturnsCopy(t *testing.T) {
	layout := testLayout(t.TempDir())
	service := newTestService(layout)
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.New(context.Background(), "工程"); err != nil {
		t.Fatal(err)
	}
	first, err := service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	first.Name = "被外部修改"
	second, err := service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Name == "被外部修改" {
		t.Fatal("Current returned a mutable internal reference")
	}
}

func TestServiceSaveReturnsDeepCopy(t *testing.T) {
	service := newTestService(testLayout(t.TempDir()))
	if err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.New(context.Background(), "工程"); err != nil {
		t.Fatal(err)
	}
	projectValue, err := service.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	projectValue.Nodes = []entity.Node{{
		ID: "n_saved", Type: valueobject.NodeTypeTopic, Title: "保存值",
		Params: entity.NodeParams{Idea: "original"}, Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "artifact"}},
	}}
	projectValue.Revision++
	saved, err := service.Save(context.Background(), projectValue)
	if err != nil {
		t.Fatal(err)
	}
	saved.Nodes[0].Title = "调用方修改"
	saved.Nodes[0].Params.Idea = "changed"
	saved.Nodes[0].Rounds[0].ArtifactID = "changed"
	if service.active.Nodes[0].Title != "保存值" || service.active.Nodes[0].Params.Idea != "original" || service.active.Nodes[0].Rounds[0].ArtifactID != "artifact" {
		t.Fatalf("Save return value aliases active project: %+v", service.active.Nodes[0])
	}
}

func writeSessionsFile(t *testing.T, layout valueobject.Layout) {
	t.Helper()
	file := valueobject.SessionsFile{
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
	if err := persistence.NewSessionStore(layout.SessionsFile).Save(context.Background(), file); err != nil {
		t.Fatal(err)
	}
}
