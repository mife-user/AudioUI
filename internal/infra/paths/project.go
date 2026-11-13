package paths

import (
	"fmt"
	"os"
	"path/filepath"

	"audio-ui/internal/domain/valueobject"
)

// ProjectPaths contains every path owned by one project directory.
type ProjectPaths struct {
	ProjectID    string
	Dir          string
	ProjectFile  string
	PendingFile  string
	PendingDir   string
	ArtifactsDir string
	RunsFile     string
	RunsDir      string
	TreesDir      string
	WorkspaceDir  string
}

// ResolveProject validates the project id and derives its directory layout.
func ResolveProject(layout valueobject.Layout, projectID string) (ProjectPaths, error) {
	if err := valueobject.ValidateID(projectID); err != nil {
		return ProjectPaths{}, err
	}
	dir := filepath.Join(layout.ProjectsDir, projectID)
	return ProjectPaths{
		ProjectID:    projectID,
		Dir:          dir,
		ProjectFile:  filepath.Join(dir, "project.json"),
		PendingFile:  filepath.Join(dir, "pending.json"),
		PendingDir:   filepath.Join(dir, "pending"),
		ArtifactsDir: filepath.Join(dir, "artifacts"),
		RunsFile:     filepath.Join(dir, "runs.json"),
		RunsDir:      filepath.Join(dir, "runs"),
		TreesDir:     filepath.Join(dir, "trees"),
		WorkspaceDir: filepath.Join(dir, "workspace"),
	}, nil
}

// TreeDir returns the cwd directory for one topic tree.
func (p ProjectPaths) TreeDir(rootTopicID string) (string, error) {
	if err := valueobject.ValidateID(rootTopicID); err != nil {
		return "", err
	}
	return filepath.Join(p.TreesDir, rootTopicID), nil
}

// StepDir returns the node increment directory for one topic tree.
func (p ProjectPaths) StepDir(rootTopicID, nodeID string) (string, error) {
	if err := valueobject.ValidateID(nodeID); err != nil {
		return "", err
	}
	treeDir, err := p.TreeDir(rootTopicID)
	if err != nil {
		return "", err
	}
	return filepath.Join(treeDir, "steps", nodeID), nil
}

func (p ProjectPaths) WorkspaceStepDir(nodeID string) (string, error) {
	if err := valueobject.ValidateID(nodeID); err != nil {
		return "", err
	}
	return filepath.Join(p.WorkspaceDir, "steps", nodeID), nil
}

// IsReparsePoint reports whether path is a symlink or Windows junction.
func IsReparsePoint(path string) (bool, error) {
	return isReparsePoint(path)
}

// EnsureProjectDirs creates the project-owned directories and verifies they
// are writable. It is used when creating a project or a migration staging tree.
func EnsureProjectDirs(p ProjectPaths) error {
	for _, dir := range []string{p.Dir, p.PendingDir, p.ArtifactsDir, p.RunsDir, p.TreesDir, p.WorkspaceDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("无法创建目录 %s: %w", dir, err)
		}
		if err := probeWritable(dir); err != nil {
			return fmt.Errorf("目录不可写 %s: %w", dir, err)
		}
	}
	return nil
}
