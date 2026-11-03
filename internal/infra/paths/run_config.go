package paths

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"audio-ui/internal/domain/valueobject"
)

// RunConfigStore places MCP bearer credentials in a disposable directory for
// one run. Callers must remove that directory when the run reaches a terminal
// state.
type RunConfigStore struct {
	layout valueobject.Layout
}

func NewRunConfigStore(layout valueobject.Layout) *RunConfigStore {
	return &RunConfigStore{layout: layout}
}

func (s *RunConfigStore) Prepare(ctx context.Context, projectID, runID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := valueobject.ValidateID(projectID); err != nil {
		return "", fmt.Errorf("invalid project id: %w", err)
	}
	if err := valueobject.ValidateID(runID); err != nil {
		return "", fmt.Errorf("invalid run id: %w", err)
	}
	if s.layout.Root == "" || s.layout.AgentDir == "" {
		return "", fmt.Errorf("run config root is not configured")
	}
	dir := filepath.Join(s.layout.AgentDir, projectID, runID)
	if err := ensureSecureDirectory(s.layout.Root, dir); err != nil {
		return "", err
	}
	return filepath.Join(dir, "mcp.json"), nil
}

func (s *RunConfigStore) Cleanup(ctx context.Context, projectID, runID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := valueobject.ValidateID(projectID); err != nil {
		return fmt.Errorf("invalid project id: %w", err)
	}
	if err := valueobject.ValidateID(runID); err != nil {
		return fmt.Errorf("invalid run id: %w", err)
	}
	if s.layout.Root == "" || s.layout.AgentDir == "" {
		return fmt.Errorf("run config root is not configured")
	}
	dir := filepath.Join(s.layout.AgentDir, projectID, runID)
	if err := inspectSecureDirectory(s.layout.Root, dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(dir)
}

func ensureSecureDirectory(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if !within(rootAbs, targetAbs) {
		return fmt.Errorf("%w: run config escapes application root", ErrPathDenied)
	}
	if err := inspectSecureDirectory(rootAbs, targetAbs); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return err
	}
	current := rootAbs
	for _, part := range splitPath(rel) {
		current = filepath.Join(current, part)
		if _, err := os.Lstat(current); os.IsNotExist(err) {
			if err := os.Mkdir(current, 0o700); err != nil && !os.IsExist(err) {
				return err
			}
		} else if err != nil {
			return err
		}
		if err := inspectDirectory(current); err != nil {
			return err
		}
	}
	return nil
}

func inspectSecureDirectory(root, target string) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	if !within(rootAbs, targetAbs) {
		return fmt.Errorf("%w: run config escapes application root", ErrPathDenied)
	}
	if err := inspectDirectory(rootAbs); err != nil {
		return err
	}
	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return err
	}
	current := rootAbs
	for _, part := range splitPath(rel) {
		current = filepath.Join(current, part)
		if err := inspectDirectory(current); err != nil {
			return err
		}
	}
	return nil
}

func inspectDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("run config path is not a directory: %s", path)
	}
	reparse, err := IsReparsePoint(path)
	if err != nil {
		return err
	}
	if reparse {
		return fmt.Errorf("%w: run config path is a reparse point", ErrPathDenied)
	}
	return nil
}

func splitPath(path string) []string {
	clean := filepath.Clean(path)
	if clean == "." || clean == "" {
		return nil
	}
	parts := []string{}
	for clean != "." && clean != string(filepath.Separator) {
		part := filepath.Base(clean)
		parts = append([]string{part}, parts...)
		parent := filepath.Dir(clean)
		if parent == clean {
			break
		}
		clean = parent
	}
	return parts
}
