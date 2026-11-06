package port

import (
	"context"
	"errors"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

// ErrMigrationFailed is the public code for a failed M1 migration.
var ErrMigrationFailed = errors.New("MIGRATION_FAILED")

// ErrUnsupportedVersion reports a persisted project format this build cannot load.
var ErrUnsupportedVersion = errors.New("UNSUPPORTED_VERSION")

// ErrPendingNotFound is returned when a pending id is absent from the project index.
var ErrPendingNotFound = errors.New("PENDING_NOT_FOUND")

// ConfigStore loads and saves data/config.json.
type ConfigStore interface {
	Load(ctx context.Context) (valueobject.AppConfig, error)
	Save(ctx context.Context, config valueobject.AppConfig) error
}

// ProjectStore manages project directories and project.json documents.
type ProjectStore interface {
	List(ctx context.Context) ([]entity.ProjectSummary, error)
	Create(ctx context.Context, project entity.Project) error
	Save(ctx context.Context, project entity.Project) error
	Load(ctx context.Context, id string) (entity.Project, error)
	Exists(ctx context.Context, id string) (bool, error)
}

// RunStore manages runs.json and runs/<runId>.json for one project.
type RunStore interface {
	List(ctx context.Context) ([]entity.RunSummary, error)
	Load(ctx context.Context, id string) (entity.RunRecord, error)
	Save(ctx context.Context, run entity.RunRecord) error
}

// RunStoreProvider returns the run history store for a project.
type RunStoreProvider interface {
	ForProject(ctx context.Context, projectID string) (RunStore, error)
}

// PendingStore manages pending.json and pending/<pendingId>.json for one project.
type PendingStore interface {
	List(ctx context.Context) ([]entity.Pending, error)
	Load(ctx context.Context, id string) (entity.Pending, error)
	GetPayload(ctx context.Context, id string) ([]byte, error)
	WritePayload(ctx context.Context, id string, payload []byte) error
	Save(ctx context.Context, pending entity.Pending, payload []byte) error
	SaveTransaction(ctx context.Context, transaction entity.PendingTransaction) error
	LoadTransaction(ctx context.Context) (entity.PendingTransaction, bool, error)
	DeleteTransaction(ctx context.Context) error
}

type PendingStoreProvider interface {
	ForProject(ctx context.Context, projectID string) (PendingStore, error)
}

// ArtifactStore manages immutable, content-addressed artifacts for one project.
type ArtifactStore interface {
	Put(ctx context.Context, payload []byte, meta entity.ArtifactMeta) (entity.Artifact, error)
	Get(ctx context.Context, id string) (entity.Artifact, []byte, error)
	Exists(ctx context.Context, id string) (bool, error)
}

// ArtifactStoreProvider returns the per-project artifact store.
type ArtifactStoreProvider interface {
	ForProject(ctx context.Context, projectID string) (ArtifactStore, error)
}

// ProjectSession is the active project owned by service/project.
type ProjectSession interface {
	Current(ctx context.Context) (entity.Project, error)
	Save(ctx context.Context, project entity.Project) (entity.Project, error)
	WithExclusive(ctx context.Context, fn func(ProjectAccess) error) error
}

// ProjectAccess is valid only while ProjectSession.WithExclusive holds its project operation lock.
type ProjectAccess interface {
	Current(ctx context.Context) (entity.Project, error)
	Save(ctx context.Context, project entity.Project) (entity.Project, error)
}

// MigrationResult reports an M1 migration attempt. Migrated is false when
// there is nothing to migrate or the target project already exists.
type MigrationResult struct {
	Project     entity.Project
	ImportedRun *entity.RunRecord
	Migrated    bool
}

// M1Migrator imports data/sessions.json and the legacy workspace once.
type M1Migrator interface {
	Migrate(ctx context.Context) (MigrationResult, error)
}
