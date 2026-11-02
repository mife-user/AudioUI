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
)

type Options struct {
	Layout   valueobject.Layout
	Config   port.ConfigStore
	Projects port.ProjectStore
	Migrator port.M1Migrator
	Logf     func(format string, args ...any)
}

type Service struct {
	opts Options

	opMu sync.Mutex
	mu   sync.Mutex

	active    *entity.Project
	config    valueobject.AppConfig
	configErr error
}

func New(opts Options) *Service {
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}
	return &Service{opts: opts}
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
			loadErr = fmt.Errorf("加载上次工程 %s 失败: %w", config.LastProjectID, err)
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
	return project.Clone(), nil
}

func (s *Service) Open(ctx context.Context, id string) (entity.Project, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	id = strings.TrimSpace(id)
	if err := valueobject.ValidateID(id); err != nil {
		return entity.Project{}, fmt.Errorf("%w: %v", ErrProjectIDInvalid, err)
	}
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
	return project.Clone(), nil
}

func (s *Service) Current(ctx context.Context) (entity.Project, error) {
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.configErr; err != nil {
		return entity.Project{}, err
	}
	if s.active == nil {
		return entity.Project{}, ErrNoActiveProject
	}
	return s.active.Clone(), nil
}

// Save persists a command-layer mutation of the active project and updates
// the in-memory copy. Revision policy belongs to the command layer.
func (s *Service) Save(ctx context.Context, project entity.Project) (entity.Project, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
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
	return saved, nil
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
