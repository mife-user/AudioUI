package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

const (
	migratedProjectID      = "p_m1_migration"
	migratedRootID         = "n_m1_migration"
	migratedRunID          = "r_m1_import"
	migratedName           = "M1 迁移"
	migrationMarkerVersion = 1
)

type migrationMarker struct {
	Version    int    `json:"version"`
	ProjectID  string `json:"projectId"`
	MigratedAt string `json:"migratedAt"`
}

// M1Migrator imports M1 sessions.json and workspace into one default project.
// It is idempotent: data/backup/m1/migration.json is the completion marker,
// and originals are copied (not moved) into data/backup/m1.
type M1Migrator struct {
	layout valueobject.Layout
	now    func() time.Time
}

func NewM1Migrator(layout valueobject.Layout) *M1Migrator {
	return &M1Migrator{layout: layout, now: time.Now}
}

func NewM1MigratorWithNow(layout valueobject.Layout, now func() time.Time) *M1Migrator {
	if now == nil {
		now = time.Now
	}
	return &M1Migrator{layout: layout, now: now}
}

func (m *M1Migrator) Migrate(ctx context.Context) (port.MigrationResult, error) {
	if err := ctx.Err(); err != nil {
		return port.MigrationResult{}, err
	}
	if _, err := os.Stat(m.layout.SessionsFile); err != nil {
		if os.IsNotExist(err) {
			return port.MigrationResult{}, nil
		}
		return port.MigrationResult{}, migrationError(err)
	}

	projectPaths, err := paths.ResolveProject(m.layout, migratedProjectID)
	if err != nil {
		return port.MigrationResult{}, migrationError(err)
	}
	marker, hasMarker, err := m.readMarker()
	if err != nil {
		return port.MigrationResult{}, migrationError(err)
	}
	if hasMarker {
		if marker.Version != migrationMarkerVersion || marker.ProjectID != migratedProjectID {
			return port.MigrationResult{}, migrationError(fmt.Errorf("迁移标记无效: %+v", marker))
		}
	}
	projectExists, err := fileExists(projectPaths.ProjectFile)
	if err != nil {
		return port.MigrationResult{}, migrationError(err)
	}
	if hasMarker {
		if !projectExists {
			// The migration already ran; the project was removed later.
			// Do not recreate it from a still-present legacy sessions.json.
			return port.MigrationResult{}, nil
		}
		project, loadErr := NewProjectStore(m.layout).Load(ctx, migratedProjectID)
		if loadErr != nil {
			return port.MigrationResult{}, migrationError(loadErr)
		}
		return port.MigrationResult{Project: project}, nil
	}
	if projectExists {
		project, loadErr := NewProjectStore(m.layout).Load(ctx, migratedProjectID)
		if loadErr != nil {
			return port.MigrationResult{}, migrationError(loadErr)
		}
		if err := m.writeMarker(); err != nil {
			return port.MigrationResult{}, migrationError(err)
		}
		return port.MigrationResult{Project: project}, nil
	}

	sessions, err := NewSessionStore(m.layout.SessionsFile).Load(ctx)
	if err != nil {
		return port.MigrationResult{}, migrationError(err)
	}

	if err := m.backupLegacy(ctx); err != nil {
		return port.MigrationResult{}, migrationError(err)
	}

	project := migratedProject()
	if err := os.MkdirAll(m.layout.ProjectsDir, 0o755); err != nil {
		return port.MigrationResult{}, migrationError(err)
	}
	staging, err := os.MkdirTemp(m.layout.ProjectsDir, ".m1-migration-*")
	if err != nil {
		return port.MigrationResult{}, migrationError(err)
	}
	defer os.RemoveAll(staging)

	if err := writeProjectSkeleton(staging, project); err != nil {
		return port.MigrationResult{}, migrationError(err)
	}
	if err := m.copyLegacyWorkspace(ctx, staging); err != nil {
		return port.MigrationResult{}, migrationError(err)
	}

	var imported *entity.RunRecord
	if record, ok := sessions.Chains[sessions.ActiveChainID]; ok {
		run := importedRun(record, m.now)
		if err := NewRunStore(staging).Save(ctx, run); err != nil {
			return port.MigrationResult{}, migrationError(err)
		}
		imported = &run
	}

	if err := os.Rename(staging, projectPaths.Dir); err != nil {
		if exists, statErr := fileExists(projectPaths.ProjectFile); statErr == nil && exists {
			existing, loadErr := NewProjectStore(m.layout).Load(ctx, migratedProjectID)
			if loadErr != nil {
				return port.MigrationResult{}, migrationError(loadErr)
			}
			if markerErr := m.writeMarker(); markerErr != nil {
				return port.MigrationResult{}, migrationError(markerErr)
			}
			return port.MigrationResult{Project: existing}, nil
		}
		return port.MigrationResult{}, migrationError(err)
	}
	if err := m.writeMarker(); err != nil {
		return port.MigrationResult{}, migrationError(err)
	}
	return port.MigrationResult{Project: project, ImportedRun: imported, Migrated: true}, nil
}

func (m *M1Migrator) markerPath() string {
	return filepath.Join(m.layout.BackupDir, "migration.json")
}

func (m *M1Migrator) readMarker() (migrationMarker, bool, error) {
	var marker migrationMarker
	found, err := readJSONFile(m.markerPath(), &marker)
	if err != nil {
		return migrationMarker{}, false, err
	}
	return marker, found, nil
}

func (m *M1Migrator) writeMarker() error {
	return writeJSONAtomic(m.markerPath(), migrationMarker{
		Version:    migrationMarkerVersion,
		ProjectID:  migratedProjectID,
		MigratedAt: m.now().Format(time.RFC3339),
	})
}

func fileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (m *M1Migrator) backupLegacy(ctx context.Context) error {
	if err := os.MkdirAll(m.layout.BackupDir, 0o755); err != nil {
		return err
	}
	if err := copyFileAtomic(ctx, m.layout.SessionsFile, filepath.Join(m.layout.BackupDir, "sessions.json"), 0o644); err != nil {
		return err
	}
	info, err := os.Stat(m.layout.WorkspaceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("旧 workspace 不是目录: %s", m.layout.WorkspaceDir)
	}
	return copyTree(ctx, m.layout.WorkspaceDir, filepath.Join(m.layout.BackupDir, "workspace", "chain_default"))
}

func (m *M1Migrator) copyLegacyWorkspace(ctx context.Context, staging string) error {
	info, err := os.Stat(m.layout.WorkspaceDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("旧 workspace 不是目录: %s", m.layout.WorkspaceDir)
	}
	target := filepath.Join(staging, "trees", migratedRootID, "steps", migratedRootID)
	return copyTree(ctx, m.layout.WorkspaceDir, target)
}

func migratedProject() entity.Project {
	return entity.Project{
		Version:      valueobject.ProjectFileVersion,
		ID:           migratedProjectID,
		Name:         migratedName,
		Revision:     1,
		ActiveNodeID: migratedRootID,
		Nodes: []entity.Node{{
			ID:       migratedRootID,
			Type:     valueobject.NodeTypeTopic,
			Title:    migratedName,
			Position: entity.Position{},
			Params:   entity.NodeParams{},
			Rounds:   []entity.Round{},
		}},
	}
}

func importedRun(record valueobject.SessionRecord, now func() time.Time) entity.RunRecord {
	startedAt := record.UpdatedAt
	if startedAt == "" {
		startedAt = now().Format(time.RFC3339)
	}
	transcript := append([]valueobject.TranscriptItem{}, record.Transcript...)
	if len(transcript) > valueobject.MaxRunTranscriptItems {
		transcript = transcript[len(transcript)-valueobject.MaxRunTranscriptItems:]
	}
	for i := range transcript {
		transcript[i].Streaming = false
	}
	return entity.RunRecord{
		RunSummary: entity.RunSummary{
			ID:             migratedRunID,
			TreeID:         migratedRootID,
			NodeID:         migratedRootID,
			ChainID:        "c_" + migratedRootID,
			ChainPath:      []string{migratedRootID},
			Status:         valueobject.RunImported,
			ContextVersion: valueobject.DefaultContextVersion,
			StartedAt:      startedAt,
			FinishedAt:     startedAt,
		},
		Transcript: transcript,
	}
}

func migrationError(err error) error {
	return fmt.Errorf("%w: %v", port.ErrMigrationFailed, err)
}
