package persistence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

func fixedNow() time.Time {
	return time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
}

func writeM1Sessions(t *testing.T, layout valueobject.Layout) {
	t.Helper()
	file := valueobject.SessionsFile{
		Version:       valueobject.SessionFileVersion,
		ActiveChainID: valueobject.DefaultChainID,
		Chains: map[string]valueobject.SessionRecord{
			valueobject.DefaultChainID: {
				Adapter:   "claude",
				DisplayID: "active",
				Status:    valueobject.SessionIdle,
				UpdatedAt: "2026-10-31T10:00:00Z",
				Transcript: []valueobject.TranscriptItem{{
					ID: "active-1", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "旧对话", Streaming: true,
				}},
			},
			"chain_other": {
				Adapter:   "claude",
				DisplayID: "other",
				Status:    valueobject.SessionIdle,
				UpdatedAt: "2026-10-31T11:00:00Z",
				Transcript: []valueobject.TranscriptItem{{
					ID: "other-1", Kind: valueobject.TranscriptMessage, Role: "assistant", Text: "不应导入",
				}},
			},
		},
	}
	if err := NewSessionStore(layout.SessionsFile).Save(context.Background(), file); err != nil {
		t.Fatal(err)
	}
}

func TestM1MigratorNoSessionsIsNoop(t *testing.T) {
	layout := testLayout(t.TempDir())
	result, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Migrated || result.Project.ID != "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(layout.ProjectsDir); !os.IsNotExist(err) {
		t.Fatalf("projects dir was created: %v", err)
	}
}

func TestM1MigratorImportsActiveChainAndWorkspace(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeM1Sessions(t, layout)
	writeTestFile(t, filepath.Join(layout.WorkspaceDir, "notes.txt"), "旧工作区")
	originalSessions, err := os.ReadFile(layout.SessionsFile)
	if err != nil {
		t.Fatal(err)
	}

	result, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Migrated {
		t.Fatal("Migrated = false")
	}
	if result.Project.ID != migratedProjectID || result.Project.Name != migratedName || result.Project.Version != valueobject.ProjectFileVersion {
		t.Fatalf("project = %+v", result.Project)
	}
	if len(result.Project.Nodes) != 1 || result.Project.Nodes[0].ID != migratedRootID ||
		result.Project.Nodes[0].Type != valueobject.NodeTypeTopic || result.Project.Nodes[0].Title != migratedName {
		t.Fatalf("nodes = %+v", result.Project.Nodes)
	}
	if result.Project.ActiveNodeID != migratedRootID {
		t.Fatalf("activeNodeId = %q", result.Project.ActiveNodeID)
	}

	loaded, err := NewProjectStore(layout).Load(context.Background(), migratedProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != migratedName {
		t.Fatalf("loaded name = %q", loaded.Name)
	}

	runs, err := NewRunStore(filepath.Join(layout.ProjectsDir, migratedProjectID)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != valueobject.RunImported || runs[0].NodeID != migratedRootID {
		t.Fatalf("runs = %+v", runs)
	}
	if len(runs[0].ChainPath) != 1 || runs[0].ChainPath[0] != migratedRootID {
		t.Fatalf("chainPath = %+v", runs[0].ChainPath)
	}
	detail, err := NewRunStore(filepath.Join(layout.ProjectsDir, migratedProjectID)).Load(context.Background(), migratedRunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Transcript) != 1 || detail.Transcript[0].Text != "旧对话" || detail.Transcript[0].Streaming {
		t.Fatalf("transcript = %+v", detail.Transcript)
	}
	if detail.StartedAt != "2026-10-31T10:00:00Z" {
		t.Fatalf("startedAt = %q", detail.StartedAt)
	}

	stepFile := filepath.Join(layout.ProjectsDir, migratedProjectID, "trees", migratedRootID, "steps", migratedRootID, "notes.txt")
	data, err := os.ReadFile(stepFile)
	if err != nil {
		t.Fatalf("workspace copy missing: %v", err)
	}
	if string(data) != "旧工作区" {
		t.Fatalf("workspace copy = %q", data)
	}

	backupSessions, err := os.ReadFile(filepath.Join(layout.BackupDir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(backupSessions) != string(originalSessions) {
		t.Fatal("backup sessions differ from original")
	}
	var marker migrationMarker
	markerFound, err := readJSONFile(filepath.Join(layout.BackupDir, "migration.json"), &marker)
	if err != nil || !markerFound {
		t.Fatalf("migration marker missing: found=%v err=%v", markerFound, err)
	}
	if marker.Version != migrationMarkerVersion || marker.ProjectID != migratedProjectID {
		t.Fatalf("marker = %+v", marker)
	}
	backupWorkspace, err := os.ReadFile(filepath.Join(layout.BackupDir, "workspace", "chain_default", "notes.txt"))
	if err != nil {
		t.Fatalf("backup workspace missing: %v", err)
	}
	if string(backupWorkspace) != "旧工作区" {
		t.Fatalf("backup workspace = %q", backupWorkspace)
	}

	currentSessions, err := os.ReadFile(layout.SessionsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(currentSessions) != string(originalSessions) {
		t.Fatal("original sessions.json was modified")
	}
	if _, err := os.Stat(filepath.Join(layout.WorkspaceDir, "notes.txt")); err != nil {
		t.Fatalf("original workspace was removed: %v", err)
	}
}

func TestM1MigratorIsIdempotent(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeM1Sessions(t, layout)
	migrator := NewM1MigratorWithNow(layout, fixedNow)
	first, err := migrator.Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !first.Migrated {
		t.Fatal("first migration did not run")
	}
	second, err := migrator.Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Migrated {
		t.Fatal("second migration ran again")
	}
	if second.Project.ID != first.Project.ID {
		t.Fatalf("project id changed: %q -> %q", first.Project.ID, second.Project.ID)
	}
	runs, err := NewRunStore(filepath.Join(layout.ProjectsDir, migratedProjectID)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestM1MigratorDoesNotReimportAfterProjectRemoval(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeM1Sessions(t, layout)
	migrator := NewM1MigratorWithNow(layout, fixedNow)
	if _, err := migrator.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(layout.ProjectsDir, migratedProjectID)); err != nil {
		t.Fatal(err)
	}
	result, err := migrator.Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Migrated || result.Project.ID != "" {
		t.Fatalf("result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(layout.ProjectsDir, migratedProjectID)); !os.IsNotExist(err) {
		t.Fatalf("migration recreated a removed project: %v", err)
	}
}

func TestM1MigratorCorruptMarkerFails(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeM1Sessions(t, layout)
	writeTestFile(t, filepath.Join(layout.BackupDir, "migration.json"), "{")
	_, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background())
	if !errors.Is(err, port.ErrMigrationFailed) {
		t.Fatalf("error = %v, want MIGRATION_FAILED", err)
	}
	if _, err := os.Stat(filepath.Join(layout.ProjectsDir, migratedProjectID)); !os.IsNotExist(err) {
		t.Fatalf("project created with corrupt marker: %v", err)
	}
}

func TestM1MigratorDoesNotOverwriteExistingProject(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeM1Sessions(t, layout)
	existing := sampleProject(migratedProjectID)
	existing.Name = "用户工程"
	if err := NewProjectStore(layout).Create(context.Background(), existing); err != nil {
		t.Fatal(err)
	}
	result, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Migrated {
		t.Fatal("migration overwrote an existing project")
	}
	loaded, err := NewProjectStore(layout).Load(context.Background(), migratedProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "用户工程" {
		t.Fatalf("project name = %q", loaded.Name)
	}
	if exists, err := fileExists(filepath.Join(layout.BackupDir, "migration.json")); err != nil || !exists {
		t.Fatalf("migration marker not written: exists=%v err=%v", exists, err)
	}
}

func TestM1MigratorSkipsUnsupportedExistingDestination(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeM1Sessions(t, layout)
	old := migratedProject()
	old.Version = 1
	writeJSONFileForTest(t, filepath.Join(layout.ProjectsDir, migratedProjectID, "project.json"), old)

	result, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Migrated || result.Project.ID != "" {
		t.Fatalf("result = %+v, want skipped unsupported destination", result)
	}
	var persisted entity.Project
	found, err := readJSONFile(filepath.Join(layout.ProjectsDir, migratedProjectID, "project.json"), &persisted)
	if err != nil || !found || persisted.Version != 1 {
		t.Fatalf("old project changed: found=%v project=%+v err=%v", found, persisted, err)
	}
	if _, err := os.Stat(layout.SessionsFile); err != nil {
		t.Fatalf("M1 source was removed: %v", err)
	}
}

func TestM1MigratorCorruptSessionsFailsExplicitly(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeTestFile(t, layout.SessionsFile, "{")
	_, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background())
	if !errors.Is(err, port.ErrMigrationFailed) {
		t.Fatalf("error = %v, want MIGRATION_FAILED", err)
	}
	if _, statErr := os.Stat(filepath.Join(layout.ProjectsDir, migratedProjectID)); !os.IsNotExist(statErr) {
		t.Fatalf("project was created after corrupt sessions: %v", statErr)
	}
	if data, readErr := os.ReadFile(layout.SessionsFile); readErr != nil || string(data) != "{" {
		t.Fatalf("corrupt sessions changed: %q %v", data, readErr)
	}
}

func TestM1MigratorFailureLeavesNoProject(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeM1Sessions(t, layout)
	writeTestFile(t, layout.BackupDir, "not a directory")
	_, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background())
	if !errors.Is(err, port.ErrMigrationFailed) {
		t.Fatalf("error = %v, want MIGRATION_FAILED", err)
	}
	if _, statErr := os.Stat(filepath.Join(layout.ProjectsDir, migratedProjectID)); !os.IsNotExist(statErr) {
		t.Fatalf("half-created project remains: %v", statErr)
	}
}

func TestM1MigratorWithoutActiveRecord(t *testing.T) {
	layout := testLayout(t.TempDir())
	file := valueobject.SessionsFile{
		Version: valueobject.SessionFileVersion, ActiveChainID: valueobject.DefaultChainID,
		Chains: map[string]valueobject.SessionRecord{},
	}
	if err := NewSessionStore(layout.SessionsFile).Save(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	result, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Migrated || result.ImportedRun != nil {
		t.Fatalf("result = %+v", result)
	}
	runs, err := NewRunStore(filepath.Join(layout.ProjectsDir, migratedProjectID)).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestM1MigratorWithoutWorkspace(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeM1Sessions(t, layout)
	result, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Migrated {
		t.Fatal("Migrated = false")
	}
	if _, err := os.Stat(filepath.Join(layout.BackupDir, "workspace")); !os.IsNotExist(err) {
		t.Fatalf("backup workspace should not exist: %v", err)
	}
}

func TestM1MigratorHonorsContextCancellation(t *testing.T) {
	layout := testLayout(t.TempDir())
	writeM1Sessions(t, layout)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(layout.ProjectsDir, migratedProjectID)); !os.IsNotExist(err) {
		t.Fatalf("project created after cancellation: %v", err)
	}
}

func TestMigratorAllPathsStayUnderPortableRoot(t *testing.T) {
	root := t.TempDir()
	layout := testLayout(root)
	writeM1Sessions(t, layout)
	writeTestFile(t, filepath.Join(layout.WorkspaceDir, "a.txt"), "a")
	if _, err := NewM1MigratorWithNow(layout, fixedNow).Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(layout.ProjectsDir, migratedProjectID),
		layout.BackupDir,
	} {
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || filepath.IsAbs(rel) {
			t.Fatalf("path %q escapes root %q", path, root)
		}
	}
}
