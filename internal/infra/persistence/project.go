package persistence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

// ProjectStore reads and writes project.json files under data/projects.
type ProjectStore struct {
	layout valueobject.Layout
	mu     sync.Mutex
}

func NewProjectStore(layout valueobject.Layout) *ProjectStore {
	return &ProjectStore{layout: layout}
}

func (s *ProjectStore) List(ctx context.Context) ([]entity.ProjectSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.layout.ProjectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []entity.ProjectSummary{}, nil
		}
		return nil, err
	}
	summaries := make([]entity.ProjectSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		project, err := s.loadLocked(entry.Name())
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, entity.ProjectSummary{
			ID: project.ID, Name: project.Name, Revision: project.Revision,
		})
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		if summaries[i].Name != summaries[j].Name {
			return summaries[i].Name < summaries[j].Name
		}
		return summaries[i].ID < summaries[j].ID
	})
	return summaries, nil
}

func (s *ProjectStore) Create(ctx context.Context, project entity.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	project = normalizeProject(project)
	if err := validateProject(project); err != nil {
		return err
	}
	projectPaths, err := paths.ResolveProject(s.layout, project.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(projectPaths.Dir); err == nil {
		return fmt.Errorf("%w: %s", ErrProjectExists, project.ID)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(s.layout.ProjectsDir, 0o755); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(s.layout.ProjectsDir, ".creating-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := writeProjectSkeleton(staging, project); err != nil {
		return err
	}
	if err := os.Rename(staging, projectPaths.Dir); err != nil {
		if _, statErr := os.Stat(projectPaths.Dir); statErr == nil {
			return fmt.Errorf("%w: %s", ErrProjectExists, project.ID)
		}
		return err
	}
	return nil
}

func (s *ProjectStore) Save(ctx context.Context, project entity.Project) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	project = normalizeProject(project)
	if err := validateProject(project); err != nil {
		return err
	}
	projectPaths, err := paths.ResolveProject(s.layout, project.ID)
	if err != nil {
		return err
	}
	if _, err := os.Stat(projectPaths.ProjectFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s", ErrProjectNotFound, project.ID)
		}
		return err
	}
	if err := writeJSONAtomic(projectPaths.ProjectFile, project); err != nil {
		return fmt.Errorf("写入 %s: %w", projectPaths.ProjectFile, err)
	}
	return nil
}

func (s *ProjectStore) Load(ctx context.Context, id string) (entity.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return entity.Project{}, err
	}
	return s.loadLocked(id)
}

func (s *ProjectStore) Exists(ctx context.Context, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	projectPaths, err := paths.ResolveProject(s.layout, id)
	if err != nil {
		return false, err
	}
	if _, err := os.Stat(projectPaths.ProjectFile); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *ProjectStore) loadLocked(id string) (entity.Project, error) {
	projectPaths, err := paths.ResolveProject(s.layout, id)
	if err != nil {
		return entity.Project{}, err
	}
	var project entity.Project
	found, err := readJSONFile(projectPaths.ProjectFile, &project)
	if err != nil {
		return entity.Project{}, fmt.Errorf("%w: %s: %v", ErrCorruptProject, projectPaths.ProjectFile, err)
	}
	if !found {
		return entity.Project{}, fmt.Errorf("%w: %s", ErrProjectNotFound, id)
	}
	if project.Version != valueobject.ProjectFileVersion {
		return entity.Project{}, fmt.Errorf("%w: project version %d", ErrUnsupportedVersion, project.Version)
	}
	if project.ID != id {
		return entity.Project{}, fmt.Errorf("%w: id %q does not match directory %q", ErrCorruptProject, project.ID, id)
	}
	project = normalizeProject(project)
	if err := validateProject(project); err != nil {
		return entity.Project{}, fmt.Errorf("%w: %v", ErrCorruptProject, err)
	}
	return project, nil
}

func writeProjectSkeleton(dir string, project entity.Project) error {
	projectPaths := paths.ProjectPaths{
		ProjectID:    project.ID,
		Dir:          dir,
		ProjectFile:  filepath.Join(dir, "project.json"),
		PendingFile:  filepath.Join(dir, "pending.json"),
		PendingDir:   filepath.Join(dir, "pending"),
		ArtifactsDir: filepath.Join(dir, "artifacts"),
		RunsFile:     filepath.Join(dir, "runs.json"),
		RunsDir:      filepath.Join(dir, "runs"),
		TreesDir:     filepath.Join(dir, "trees"),
	}
	if err := paths.EnsureProjectDirs(projectPaths); err != nil {
		return err
	}
	if err := writeJSONAtomic(projectPaths.ProjectFile, project); err != nil {
		return err
	}
	if err := writeJSONAtomic(projectPaths.PendingFile, entity.PendingFile{
		Version: valueobject.PendingFileVersion, Pending: []entity.Pending{},
	}); err != nil {
		return err
	}
	if err := writeJSONAtomic(projectPaths.RunsFile, entity.RunsFile{
		Version: valueobject.RunsFileVersion, Runs: []entity.RunSummary{},
	}); err != nil {
		return err
	}
	return nil
}

func normalizeProject(project entity.Project) entity.Project {
	if project.Nodes == nil {
		project.Nodes = []entity.Node{}
	}
	for i := range project.Nodes {
		if project.Nodes[i].Rounds == nil {
			project.Nodes[i].Rounds = []entity.Round{}
		}
	}
	return project
}

func validateProject(project entity.Project) error {
	if project.Version != valueobject.ProjectFileVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalidProject, project.Version)
	}
	if err := valueobject.ValidateID(project.ID); err != nil {
		return fmt.Errorf("%w: id: %v", ErrInvalidProject, err)
	}
	name := strings.TrimSpace(project.Name)
	if name == "" {
		return fmt.Errorf("%w: name is empty", ErrInvalidProject)
	}
	if utf8.RuneCountInString(project.Name) > valueobject.MaxProjectNameLength {
		return fmt.Errorf("%w: name is too long", ErrInvalidProject)
	}
	if project.Revision < 0 {
		return fmt.Errorf("%w: revision is negative", ErrInvalidProject)
	}
	seen := make(map[string]struct{}, len(project.Nodes))
	for _, node := range project.Nodes {
		if err := valueobject.ValidateID(node.ID); err != nil {
			return fmt.Errorf("%w: node id: %v", ErrInvalidProject, err)
		}
		if _, ok := seen[node.ID]; ok {
			return fmt.Errorf("%w: duplicate node id %q", ErrInvalidProject, node.ID)
		}
		seen[node.ID] = struct{}{}
		if !node.Type.Valid() {
			return fmt.Errorf("%w: node %s has invalid type %q", ErrInvalidProject, node.ID, node.Type)
		}
		if strings.TrimSpace(node.Title) == "" {
			return fmt.Errorf("%w: node %s title is empty", ErrInvalidProject, node.ID)
		}
		if utf8.RuneCountInString(node.Title) > valueobject.MaxNodeTitleLength {
			return fmt.Errorf("%w: node %s title is too long", ErrInvalidProject, node.ID)
		}
		if node.Parent != "" {
			if err := valueobject.ValidateID(node.Parent); err != nil {
				return fmt.Errorf("%w: node %s parent: %v", ErrInvalidProject, node.ID, err)
			}
		}
		if node.ParentCandidate != nil && *node.ParentCandidate < 0 {
			return fmt.Errorf("%w: node %s parentCandidate is negative", ErrInvalidProject, node.ID)
		}
		if node.Params.OutputKind != "" && !node.Params.OutputKind.Valid() {
			return fmt.Errorf("%w: node %s outputKind %q", ErrInvalidProject, node.ID, node.Params.OutputKind)
		}
		if node.Params.CandidateCount < 0 {
			return fmt.Errorf("%w: node %s candidateCount is negative", ErrInvalidProject, node.ID)
		}
		for _, round := range node.Rounds {
			if round.N <= 0 {
				return fmt.Errorf("%w: node %s round number must be positive", ErrInvalidProject, node.ID)
			}
			if !round.Kind.Valid() {
				return fmt.Errorf("%w: node %s round kind %q", ErrInvalidProject, node.ID, round.Kind)
			}
			if round.ArtifactID == "" {
				return fmt.Errorf("%w: node %s round artifact is empty", ErrInvalidProject, node.ID)
			}
		}
	}
	if project.ActiveNodeID != "" {
		if err := valueobject.ValidateID(project.ActiveNodeID); err != nil {
			return fmt.Errorf("%w: activeNodeId: %v", ErrInvalidProject, err)
		}
		if _, ok := seen[project.ActiveNodeID]; !ok {
			return fmt.Errorf("%w: activeNodeId %q not found", ErrInvalidProject, project.ActiveNodeID)
		}
	}
	return nil
}
