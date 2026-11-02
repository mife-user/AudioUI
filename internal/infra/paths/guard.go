package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrPathDenied = errors.New("PATH_DENIED")

type Guard struct {
	Root      string
	Workspace string
}

func NewGuard(root, workspace string) (*Guard, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return nil, err
	}
	return &Guard{Root: rootAbs, Workspace: workspaceAbs}, nil
}

// ResolveRead and ResolveWrite preserve the M1 single-workspace behaviour.
// M2 run paths use RunGuard to narrow these operations independently.
func (g *Guard) ResolveRead(input string) (string, error)  { return g.Resolve(input) }
func (g *Guard) ResolveWrite(input string) (string, error) { return g.Resolve(input) }

// Resolve resolves a workspace-relative path and rejects escapes, UNC/device
// paths, drive-relative paths, and protected directories.
func (g *Guard) Resolve(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", fmt.Errorf("%w: empty path", ErrPathDenied)
	}
	if strings.HasPrefix(raw, `\`) || strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("%w: UNC/device path", ErrPathDenied)
	}

	var abs string
	if len(raw) >= 2 && raw[1] == ':' {
		if !filepath.IsAbs(raw) {
			return "", fmt.Errorf("%w: drive-relative path", ErrPathDenied)
		}
		abs = filepath.Clean(raw)
	} else if filepath.IsAbs(raw) {
		return "", fmt.Errorf("%w: absolute path", ErrPathDenied)
	} else {
		cleaned := filepath.Clean(raw)
		if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("%w: path escapes workspace", ErrPathDenied)
		}
		abs = filepath.Join(g.Workspace, cleaned)
	}
	if !within(g.Workspace, abs) {
		return "", fmt.Errorf("%w: path escapes workspace", ErrPathDenied)
	}

	rel, err := filepath.Rel(g.Workspace, abs)
	if err != nil {
		return "", fmt.Errorf("%w: path escapes workspace", ErrPathDenied)
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		lower := strings.ToLower(part)
		if lower == "runtime" || lower == "artifacts" {
			return "", fmt.Errorf("%w: protected directory %s", ErrPathDenied, part)
		}
	}

	if err := g.rejectReparse(abs); err != nil {
		return "", err
	}

	// If the path (or its nearest existing parent) is a symlink/junction,
	// verify the resolved target is still inside the workspace.
	check := abs
	for {
		resolved, err := filepath.EvalSymlinks(check)
		if err == nil {
			if !within(g.Workspace, resolved) {
				return "", fmt.Errorf("%w: symlink/junction escape", ErrPathDenied)
			}
			break
		}
		parent := filepath.Dir(check)
		if parent == check || !within(g.Workspace, parent) {
			break
		}
		check = parent
	}

	return abs, nil
}

func (g *Guard) rejectReparse(abs string) error {
	if info, err := os.Lstat(g.Workspace); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%w: workspace is not a directory", ErrPathDenied)
		}
		reparse, reparseErr := isReparsePoint(g.Workspace)
		if reparseErr != nil {
			return fmt.Errorf("%w: inspect workspace: %v", ErrPathDenied, reparseErr)
		}
		if reparse {
			return fmt.Errorf("%w: workspace is a reparse point/junction", ErrPathDenied)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%w: inspect workspace: %v", ErrPathDenied, err)
	}

	rel, err := filepath.Rel(g.Workspace, abs)
	if err != nil {
		return fmt.Errorf("%w: path escapes workspace", ErrPathDenied)
	}
	current := g.Workspace
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		if _, err := os.Lstat(current); err != nil {
			break
		}
		reparse, err := isReparsePoint(current)
		if err == nil && reparse {
			return fmt.Errorf("%w: reparse point/junction", ErrPathDenied)
		}
	}
	return nil
}

func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func IsPathDenied(err error) bool {
	return errors.Is(err, ErrPathDenied)
}
