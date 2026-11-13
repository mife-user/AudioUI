package paths

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"audio-ui/internal/domain/valueobject"
)

var ErrWorkspaceScopeInvalid = errors.New("WORKSPACE_SCOPE_INVALID")

// RunGuard validates paths exactly as the agent process will interpret them
// from a topic tree cwd. Built-in reads may access only steps on chainPath,
// writes only the current node step, while Resolve intentionally retains the
// broader topic-tree boundary used by the cooperative Bash policy.
type RunGuard struct {
	tree      *Guard
	runtime   *Guard
	readRoots  []string
	readGuards []*Guard
	writeRoot  string
	currentID string
	chainPath []string
}

func NewRunGuard(treeDir, currentNodeID string, chainPath []string, sharedRuntimeDir ...string) (*RunGuard, error) {
	if currentNodeID == "" || len(chainPath) == 0 {
		return nil, fmt.Errorf("%w: active node and chain path are required", ErrWorkspaceScopeInvalid)
	}
	if err := valueobject.ValidateID(currentNodeID); err != nil {
		return nil, fmt.Errorf("%w: current node: %v", ErrWorkspaceScopeInvalid, err)
	}
	if chainPath[len(chainPath)-1] != currentNodeID {
		return nil, fmt.Errorf("%w: chain path must end at current node", ErrWorkspaceScopeInvalid)
	}

	seen := make(map[string]struct{}, len(chainPath))
	for _, nodeID := range chainPath {
		if err := valueobject.ValidateID(nodeID); err != nil {
			return nil, fmt.Errorf("%w: chain path: %v", ErrWorkspaceScopeInvalid, err)
		}
		if _, exists := seen[nodeID]; exists {
			return nil, fmt.Errorf("%w: duplicate node %s", ErrWorkspaceScopeInvalid, nodeID)
		}
		seen[nodeID] = struct{}{}
	}

	treeAbs, err := filepath.Abs(treeDir)
	if err != nil {
		return nil, fmt.Errorf("%w: tree path: %v", ErrWorkspaceScopeInvalid, err)
	}
	if err := rejectReparseAncestors(treeAbs); err != nil {
		return nil, err
	}
	info, err := os.Lstat(treeAbs)
	if err != nil {
		return nil, fmt.Errorf("%w: tree path: %v", ErrWorkspaceScopeInvalid, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: tree path is not a directory", ErrWorkspaceScopeInvalid)
	}
	reparse, err := isReparsePoint(treeAbs)
	if err != nil {
		return nil, fmt.Errorf("%w: inspect tree path: %v", ErrWorkspaceScopeInvalid, err)
	}
	if reparse {
		return nil, fmt.Errorf("%w: tree path is a reparse point/junction", ErrWorkspaceScopeInvalid)
	}

	base, err := NewGuard(treeAbs, treeAbs)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWorkspaceScopeInvalid, err)
	}
	readRoots := make([]string, 0, len(chainPath))
	for _, nodeID := range chainPath {
		readRoots = append(readRoots, filepath.Join(treeAbs, "steps", nodeID))
	}
	var runtime *Guard
	if len(sharedRuntimeDir) > 0 && sharedRuntimeDir[0] != "" {
		runtimeAbs, absErr := filepath.Abs(sharedRuntimeDir[0])
		if absErr != nil {
			return nil, fmt.Errorf("%w: runtime path: %v", ErrWorkspaceScopeInvalid, absErr)
		}
		if err := rejectReparseAncestors(runtimeAbs); err != nil {
			return nil, err
		}
		runtime, err = NewGuard(runtimeAbs, runtimeAbs)
		if err != nil {
			return nil, fmt.Errorf("%w: runtime path: %v", ErrWorkspaceScopeInvalid, err)
		}
	}
	readGuards, err := runReadGuards(readRoots)
	if err != nil {
		return nil, err
	}
	return &RunGuard{
		tree:      base,
		runtime:   runtime,
		readRoots: readRoots, readGuards: readGuards,
		writeRoot: filepath.Join(treeAbs, "steps", currentNodeID),
		currentID: currentNodeID,
		chainPath: append([]string(nil), chainPath...),
	}, nil
}

func NewRunGuardWithRoots(cwdDir, currentNodeID string, readRoots []string, writeRoot string, sharedRuntimeDir ...string) (*RunGuard, error) {
	if currentNodeID == "" {
		return nil, fmt.Errorf("%w: active node is required", ErrWorkspaceScopeInvalid)
	}
	if err := valueobject.ValidateID(currentNodeID); err != nil {
		return nil, fmt.Errorf("%w: current node: %v", ErrWorkspaceScopeInvalid, err)
	}
	cwdAbs, err := filepath.Abs(cwdDir)
	if err != nil {
		return nil, fmt.Errorf("%w: cwd path: %v", ErrWorkspaceScopeInvalid, err)
	}
	if err := validateRunDirectory(cwdAbs); err != nil {
		return nil, err
	}
	base, err := NewGuard(cwdAbs, cwdAbs)
	if err != nil {
		return nil, err
	}
	writeAbs, err := filepath.Abs(writeRoot)
	if err != nil || !within(cwdAbs, writeAbs) {
		return nil, fmt.Errorf("%w: write root must be within the run workspace", ErrWorkspaceScopeInvalid)
	}
	readAbs := make([]string, 0, len(readRoots))
	for _, root := range readRoots {
		absolute, absErr := filepath.Abs(root)
		if absErr != nil {
			return nil, fmt.Errorf("%w: read root: %v", ErrWorkspaceScopeInvalid, absErr)
		}
		if err := rejectReparseAncestors(absolute); err != nil {
			return nil, err
		}
		readAbs = append(readAbs, absolute)
	}
	readGuards, err := runReadGuards(readAbs)
	if err != nil {
		return nil, err
	}
	var runtime *Guard
	if len(sharedRuntimeDir) > 0 && sharedRuntimeDir[0] != "" {
		runtimeAbs, absErr := filepath.Abs(sharedRuntimeDir[0])
		if absErr != nil {
			return nil, fmt.Errorf("%w: runtime path: %v", ErrWorkspaceScopeInvalid, absErr)
		}
		if err := rejectReparseAncestors(runtimeAbs); err != nil {
			return nil, err
		}
		runtime, err = NewGuard(runtimeAbs, runtimeAbs)
		if err != nil {
			return nil, fmt.Errorf("%w: runtime path: %v", ErrWorkspaceScopeInvalid, err)
		}
	}
	return &RunGuard{
		tree: base, runtime: runtime, readRoots: readAbs, readGuards: readGuards,
		writeRoot: writeAbs, currentID: currentNodeID,
	}, nil
}

func runReadGuards(roots []string) ([]*Guard, error) {
	guards := make([]*Guard, 0, len(roots))
	for _, root := range roots {
		guard, err := NewGuard(root, root)
		if err != nil {
			return nil, err
		}
		guards = append(guards, guard)
	}
	return guards, nil
}

func validateRunDirectory(path string) error {
	if err := rejectReparseAncestors(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("%w: workspace path: %v", ErrWorkspaceScopeInvalid, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: workspace path is not a directory", ErrWorkspaceScopeInvalid)
	}
	reparse, err := isReparsePoint(path)
	if err != nil {
		return fmt.Errorf("%w: inspect workspace path: %v", ErrWorkspaceScopeInvalid, err)
	}
	if reparse {
		return fmt.Errorf("%w: workspace path is a reparse point/junction", ErrWorkspaceScopeInvalid)
	}
	return nil
}

// Resolve enforces only the topic-tree boundary. It is used for Bash because
// shell write targets cannot be parsed reliably in M2 (RM-017).
func (g *RunGuard) Resolve(input string) (string, error) {
	return g.tree.Resolve(input)
}

func (g *RunGuard) ResolveRead(input string) (string, error) {
	resolved, err := g.tree.Resolve(input)
	if err == nil {
		for _, root := range g.readRoots {
			if within(root, resolved) {
				return resolved, nil
			}
		}
	}
	if filepath.IsAbs(input) || filepath.VolumeName(input) != "" {
		absolute, absErr := filepath.Abs(input)
		if absErr == nil {
			for _, guard := range g.readGuards {
				if within(guard.Workspace, absolute) {
					relative, relErr := filepath.Rel(guard.Workspace, absolute)
					if relErr == nil {
						return guard.Resolve(relative)
					}
				}
			}
		}
	}
	if g.runtime != nil {
		runtimePath := input
		if !filepath.IsAbs(runtimePath) && filepath.VolumeName(runtimePath) == "" {
			runtimePath = filepath.Join(g.tree.Workspace, runtimePath)
		}
		absolute, absErr := filepath.Abs(runtimePath)
		if absErr == nil && within(g.runtime.Workspace, absolute) {
			relative, relErr := filepath.Rel(g.runtime.Workspace, absolute)
			if relErr == nil {
				if resolved, resolveErr := g.runtime.Resolve(relative); resolveErr == nil {
					return resolved, nil
				}
			}
		}
	}
	return "", fmt.Errorf("%w: read is outside the active path", ErrPathDenied)
}

func (g *RunGuard) ResolveWrite(input string) (string, error) {
	resolved, err := g.tree.Resolve(input)
	if err != nil {
		return "", err
	}
	if !within(g.writeRoot, resolved) {
		return "", fmt.Errorf("%w: write is outside steps/%s", ErrPathDenied, g.currentID)
	}
	return resolved, nil
}

func (g *RunGuard) TreeDir() string     { return g.tree.Workspace }
func (g *RunGuard) WriteDir() string    { return g.writeRoot }
func (g *RunGuard) ChainPath() []string { return append([]string(nil), g.chainPath...) }

func rejectReparseAncestors(target string) error {
	current := filepath.Clean(target)
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%w: path ancestor is not a directory: %s", ErrWorkspaceScopeInvalid, current)
			}
			reparse, inspectErr := isReparsePoint(current)
			if inspectErr != nil {
				return fmt.Errorf("%w: inspect path ancestor %s: %v", ErrWorkspaceScopeInvalid, current, inspectErr)
			}
			if reparse {
				return fmt.Errorf("%w: path ancestor is a reparse point/junction: %s", ErrWorkspaceScopeInvalid, current)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("%w: inspect path ancestor %s: %v", ErrWorkspaceScopeInvalid, current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func IsWorkspaceScopeInvalid(err error) bool {
	return errors.Is(err, ErrWorkspaceScopeInvalid)
}
