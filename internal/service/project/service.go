package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
)

var (
	ErrProjectNameInvalid   = errors.New("PROJECT_NAME_INVALID")
	ErrProjectIDInvalid     = errors.New("PROJECT_ID_INVALID")
	ErrNoActiveProject      = errors.New("NO_ACTIVE_PROJECT")
	ErrActiveProjectChanged = errors.New("ACTIVE_PROJECT_CHANGED")
	ErrRunInProgress        = errors.New("BUSY: cannot switch projects while a run is active")
)

type Options struct {
	Layout   valueobject.Layout
	Config   port.ConfigStore
	Projects port.ProjectStore
	Migrator port.M1Migrator
	OnChange func(projectID string, projectRevision int64)
	Logf     func(format string, args ...any)
}

type Service struct {
	opts Options

	opMu sync.Mutex
	mu   sync.Mutex

	active    *entity.Project
	config    valueobject.AppConfig
	configErr error
	runActive func() bool
}

func New(opts Options) *Service {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &Service{opts: opts}
}

// SetRunActiveCheck prevents switching projects while a project-scoped run is
// using its graph, workspace, and lock snapshot.
func (s *Service) SetRunActiveCheck(check func() bool) {
	s.mu.Lock()
	s.runActive = check
	s.mu.Unlock()
}

// Start loads config, runs the one-time M1 migration and restores the last
// project. A completed migration with an empty lastProjectId becomes active.
func (s *Service) Start(ctx context.Context) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	config, err := s.opts.Config.Load(ctx)
	if err != nil {
		s.mu.Lock()
		s.configErr = err
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	s.config = config
	s.mu.Unlock()

	var migrationErr error
	if s.opts.Migrator != nil {
		result, migrateErr := s.opts.Migrator.Migrate(ctx)
		if migrateErr != nil {
			migrationErr = fmt.Errorf("%w: %v（备份目录：%s）", port.ErrMigrationFailed, migrateErr, s.opts.Layout.BackupDir)
			s.opts.Logf("m1 migration failed: %v", migrateErr)
		} else if result.Project.ID != "" && config.LastProjectID == "" {
			config.LastProjectID = result.Project.ID
			if err := s.opts.Config.Save(ctx, config); err != nil {
				return fmt.Errorf("保存迁移后的 config.json: %w", err)
			}
		}
	}

	s.mu.Lock()
	s.config = config
	s.mu.Unlock()

	var loadErr error
	if config.LastProjectID != "" {
		project, err := s.opts.Projects.Load(ctx, config.LastProjectID)
		if err != nil {
			if errors.Is(err, port.ErrUnsupportedVersion) {
				s.opts.Logf("last project %s uses an unsupported format: %v", config.LastProjectID, err)
			} else {
				loadErr = fmt.Errorf("加载上次工程 %s 失败: %w", config.LastProjectID, err)
			}
		} else {
			s.mu.Lock()
			s.active = &project
			s.mu.Unlock()
		}
	}

	if migrationErr != nil || loadErr != nil {
		return errors.Join(migrationErr, loadErr)
	}
	return nil
}

func (s *Service) List(ctx context.Context) ([]entity.ProjectSummary, error) {
	return s.opts.Projects.List(ctx)
}

func (s *Service) New(ctx context.Context, name string) (entity.Project, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	if err := s.ensureCanSwitch(""); err != nil {
		return entity.Project{}, err
	}

	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > valueobject.MaxProjectNameLength {
		return entity.Project{}, ErrProjectNameInvalid
	}
	if err := s.configError(); err != nil {
		return entity.Project{}, err
	}

	id, err := valueobject.NewID(valueobject.ProjectIDPrefix)
	if err != nil {
		return entity.Project{}, err
	}
	project := entity.Project{
		Version:  valueobject.ProjectFileVersion,
		ID:       id,
		Name:     name,
		Revision: 1,
		Nodes:    []entity.Node{},
	}
	if err := s.opts.Projects.Create(ctx, project); err != nil {
		return entity.Project{}, fmt.Errorf("新建工程失败: %w", err)
	}

	if err := s.setLastProject(ctx, id); err != nil {
		return entity.Project{}, err
	}

	s.mu.Lock()
	s.active = &project
	s.mu.Unlock()
	if s.opts.OnChange != nil {
		s.opts.OnChange(project.ID, project.Revision)
	}
	return project.Clone(), nil
}

func (s *Service) Open(ctx context.Context, id string) (entity.Project, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	id = strings.TrimSpace(id)
	if err := valueobject.ValidateID(id); err != nil {
		return entity.Project{}, fmt.Errorf("%w: %v", ErrProjectIDInvalid, err)
	}
	if err := s.ensureCanSwitch(id); err != nil {
		return entity.Project{}, err
	}
	s.mu.Lock()
	previousID := ""
	if s.active != nil {
		previousID = s.active.ID
	}
	s.mu.Unlock()
	if err := s.configError(); err != nil {
		return entity.Project{}, err
	}

	project, err := s.opts.Projects.Load(ctx, id)
	if err != nil {
		return entity.Project{}, fmt.Errorf("打开工程 %s 失败: %w", id, err)
	}
	if err := s.setLastProject(ctx, id); err != nil {
		return entity.Project{}, err
	}

	s.mu.Lock()
	s.active = &project
	s.mu.Unlock()
	if previousID != project.ID && s.opts.OnChange != nil {
		s.opts.OnChange(project.ID, project.Revision)
	}
	return project.Clone(), nil
}

func (s *Service) ensureCanSwitch(targetID string) error {
	s.mu.Lock()
	check := s.runActive
	activeID := ""
	if s.active != nil {
		activeID = s.active.ID
	}
	s.mu.Unlock()
	if check != nil && check() && (targetID == "" || targetID != activeID) {
		return ErrRunInProgress
	}
	return nil
}

func (s *Service) Current(ctx context.Context) (entity.Project, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.currentLocked(ctx)
}

func (s *Service) currentLocked(ctx context.Context) (entity.Project, error) {
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	s.mu.Lock()
	if err := s.configErr; err != nil {
		s.mu.Unlock()
		return entity.Project{}, err
	}
	if s.active == nil {
		s.mu.Unlock()
		return entity.Project{}, ErrNoActiveProject
	}
	projectID := s.active.ID
	s.mu.Unlock()

	project, err := s.opts.Projects.Load(ctx, projectID)
	if err != nil {
		return entity.Project{}, err
	}
	s.mu.Lock()
	s.active = &project
	s.mu.Unlock()
	return project.Clone(), nil
}

// WithExclusive keeps project switching out of multi-file workflow commits.
func (s *Service) WithExclusive(ctx context.Context, fn func(port.ProjectAccess) error) error {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	project, err := s.currentLocked(ctx)
	if err != nil {
		return err
	}
	return fn(&projectAccess{service: s, projectID: project.ID})
}

type projectAccess struct {
	service   *Service
	projectID string
}

func (a *projectAccess) Current(ctx context.Context) (entity.Project, error) {
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	a.service.mu.Lock()
	defer a.service.mu.Unlock()
	if a.service.active == nil {
		return entity.Project{}, ErrNoActiveProject
	}
	if a.service.active.ID != a.projectID {
		return entity.Project{}, fmt.Errorf("%w: active %s, expected %s", ErrActiveProjectChanged, a.service.active.ID, a.projectID)
	}
	return a.service.active.Clone(), nil
}

func (a *projectAccess) Save(ctx context.Context, project entity.Project) (entity.Project, error) {
	if project.ID != a.projectID {
		return entity.Project{}, fmt.Errorf("%w: active %s, got %s", ErrActiveProjectChanged, a.projectID, project.ID)
	}
	return a.service.saveLocked(ctx, project)
}

// Save persists a command-layer mutation of the active project and updates
// the in-memory copy. Revision policy belongs to the command layer.
func (s *Service) Save(ctx context.Context, project entity.Project) (entity.Project, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	return s.saveLocked(ctx, project)
}

func (s *Service) saveLocked(ctx context.Context, project entity.Project) (entity.Project, error) {
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	s.mu.Lock()
	if s.configErr != nil {
		err := s.configErr
		s.mu.Unlock()
		return entity.Project{}, err
	}
	if s.active == nil {
		s.mu.Unlock()
		return entity.Project{}, ErrNoActiveProject
	}
	if s.active.ID != project.ID {
		s.mu.Unlock()
		return entity.Project{}, fmt.Errorf("%w: active %s, got %s", ErrActiveProjectChanged, s.active.ID, project.ID)
	}
	s.mu.Unlock()

	if err := s.opts.Projects.Save(ctx, project); err != nil {
		return entity.Project{}, err
	}
	saved := project.Clone()
	s.mu.Lock()
	s.active = &saved
	s.mu.Unlock()
	return saved.Clone(), nil
}

func (s *Service) Config(ctx context.Context) (valueobject.AppConfig, error) {
	if err := ctx.Err(); err != nil {
		return valueobject.AppConfig{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.configErr != nil {
		return valueobject.AppConfig{}, s.configErr
	}
	return s.config, nil
}

func (s *Service) setLastProject(ctx context.Context, id string) error {
	s.mu.Lock()
	if s.configErr != nil {
		err := s.configErr
		s.mu.Unlock()
		return err
	}
	config := s.config
	s.mu.Unlock()

	config.LastProjectID = id
	if err := s.opts.Config.Save(ctx, config); err != nil {
		return fmt.Errorf("保存 config.json 失败: %w", err)
	}
	s.mu.Lock()
	s.config = config
	s.mu.Unlock()
	return nil
}

func (s *Service) configError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.configErr
}
