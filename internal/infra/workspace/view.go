package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

type viewLayer struct {
	root  string
	guard *paths.Guard
}

// View is the logical read-only overlay for one run path. Layers are ordered
// from the current node through its ancestors, with root runtime last.
type View struct {
	projectID       string
	treeID          string
	treeDir         string
	cwd             string
	writeDir        string
	chainPath       []string
	inputNodeIDs    []string
	layers          []viewLayer
	sourceLayers    map[string]viewLayer
	inputSources    []port.WorkspaceSource
	runGuard        *paths.RunGuard
	runtimeReadPath string
	allowAbsolute   bool
}

// NewRunView derives a topic-tree workspace view. chainPath must be the real
// topic-root-to-target path; graph validation remains the workflow service's
// responsibility.
func NewRunView(layout valueobject.Layout, projectID, treeID, nodeID string, chainPath []string) (*View, error) {
	if err := valueobject.ValidateID(projectID); err != nil {
		return nil, err
	}
	if err := valueobject.ValidateID(treeID); err != nil {
		return nil, err
	}
	projectPaths, err := paths.ResolveProject(layout, projectID)
	if err != nil {
		return nil, err
	}
	treeDir, err := projectPaths.TreeDir(treeID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(layout.Root) == "" {
		return nil, fmt.Errorf("%w: application root is required", paths.ErrWorkspaceScopeInvalid)
	}
	if _, err := os.Stat(projectPaths.Dir); err != nil {
		return nil, fmt.Errorf("%w: project directory: %v", paths.ErrWorkspaceScopeInvalid, err)
	}
	if err := secureDirectoryPath(layout.Root, projectPaths.Dir, false); err != nil {
		return nil, err
	}
	if len(chainPath) == 0 || chainPath[0] != treeID || chainPath[len(chainPath)-1] != nodeID {
		return nil, fmt.Errorf("%w: chain path must start at tree root and end at target node", paths.ErrWorkspaceScopeInvalid)
	}
	if err := secureDirectoryPath(layout.Root, treeDir, true); err != nil {
		return nil, err
	}
	runtimeDir := filepath.Join(layout.Root, "runtime")
	runtimeReadPath, err := filepath.Rel(treeDir, runtimeDir)
	if err != nil {
		return nil, err
	}
	runGuard, err := paths.NewRunGuard(treeDir, nodeID, chainPath, runtimeDir)
	if err != nil {
		return nil, err
	}
	if err := secureDirectoryPath(layout.Root, filepath.Join(treeDir, "steps"), true); err != nil {
		return nil, err
	}
	for _, pathNodeID := range chainPath {
		stepDir, stepErr := projectPaths.StepDir(treeID, pathNodeID)
		if stepErr != nil {
			return nil, stepErr
		}
		if err := secureDirectoryPath(layout.Root, stepDir, true); err != nil {
			return nil, err
		}
	}

	view := &View{
		projectID: projectID, treeID: treeID,
		treeDir: runGuard.TreeDir(), cwd: runGuard.TreeDir(), writeDir: runGuard.WriteDir(),
		chainPath: runGuard.ChainPath(), inputNodeIDs: append([]string(nil), chainPath...),
		layers:       make([]viewLayer, 0, len(chainPath)+1),
		sourceLayers: make(map[string]viewLayer),
		runGuard:     runGuard, runtimeReadPath: filepath.ToSlash(runtimeReadPath),
	}
	for i := len(chainPath) - 1; i >= 0; i-- {
		stepDir, stepErr := projectPaths.StepDir(treeID, chainPath[i])
		if stepErr != nil {
			return nil, stepErr
		}
		layer, layerErr := newViewLayer(stepDir)
		if layerErr != nil {
			return nil, layerErr
		}
		view.layers = append(view.layers, layer)
	}
	if layout.RuntimeDir != "" {
		configuredRuntime, absErr := filepath.Abs(layout.RuntimeDir)
		if absErr != nil {
			return nil, absErr
		}
		expectedRuntime, absErr := filepath.Abs(runtimeDir)
		if absErr != nil {
			return nil, absErr
		}
		if filepath.Clean(configuredRuntime) != filepath.Clean(expectedRuntime) {
			return nil, fmt.Errorf("%w: shared runtime must be <root>/runtime", paths.ErrWorkspaceScopeInvalid)
		}
	}
	if err := secureDirectoryPath(layout.Root, runtimeDir, false); err != nil {
		return nil, err
	}
	layer, layerErr := newViewLayer(runtimeDir)
	if layerErr != nil {
		return nil, layerErr
	}
	view.layers = append(view.layers, layer)
	return view, nil
}

func NewSnapshotRunView(layout valueobject.Layout, projectID string, snapshot entity.RunExecutionSnapshot) (*View, error) {
	if err := valueobject.ValidateID(projectID); err != nil {
		return nil, err
	}
	if err := snapshot.Validate(); err != nil || snapshot.ProjectID != projectID || snapshot.Version < 3 || snapshot.Version > 4 {
		return nil, fmt.Errorf("%w: invalid DAG execution snapshot", paths.ErrWorkspaceScopeInvalid)
	}
	projectPaths, err := paths.ResolveProject(layout, projectID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(layout.Root) == "" {
		return nil, fmt.Errorf("%w: application root is required", paths.ErrWorkspaceScopeInvalid)
	}
	if _, err := os.Stat(projectPaths.Dir); err != nil {
		return nil, fmt.Errorf("%w: project directory: %v", paths.ErrWorkspaceScopeInvalid, err)
	}
	if err := secureDirectoryPath(layout.Root, projectPaths.Dir, false); err != nil {
		return nil, err
	}
	var treeDir, cwd, writeDir string
	if snapshot.TreeID != "" {
		treeDir, err = projectPaths.TreeDir(snapshot.TreeID)
		if err != nil {
			return nil, err
		}
		if err := secureDirectoryPath(layout.Root, treeDir, true); err != nil {
			return nil, err
		}
		cwd = treeDir
		writeDir, err = projectPaths.StepDir(snapshot.TreeID, snapshot.NodeID)
	} else {
		treeDir = projectPaths.WorkspaceDir
		if err := secureDirectoryPath(layout.Root, treeDir, true); err != nil {
			return nil, err
		}
		cwd, err = projectPaths.WorkspaceStepDir(snapshot.NodeID)
		if err == nil {
			writeDir = cwd
		}
	}
	if err != nil {
		return nil, err
	}
	if err := secureDirectoryPath(layout.Root, writeDir, true); err != nil {
		return nil, err
	}

	sourceLayers := make(map[string]viewLayer)
	inputSources := make([]port.WorkspaceSource, 0, len(snapshot.InputNodes)-1)
	readRoots := make([]string, 0, len(snapshot.InputNodes)-1)
	for _, node := range snapshot.InputNodes {
		if node.ID == snapshot.NodeID || node.Type == valueobject.NodeTypeTopic {
			continue
		}
		var stepDir string
		rootID := node.WorkspaceRootID
		if rootID == "" {
			rootID = "project"
			stepDir, err = projectPaths.WorkspaceStepDir(node.ID)
		} else {
			stepDir, err = projectPaths.StepDir(rootID, node.ID)
		}
		if err != nil {
			return nil, err
		}
		if err := secureDirectoryPath(layout.Root, stepDir, true); err != nil {
			return nil, err
		}
		layer, err := newViewLayer(stepDir)
		if err != nil {
			return nil, err
		}
		logicalRoot := "inputs/" + rootID + "/" + node.ID
		sourceLayers[logicalRoot] = layer
		readRoots = append(readRoots, stepDir)
		inputSources = append(inputSources, port.WorkspaceSource{LogicalPath: logicalRoot + "/", PhysicalPath: stepDir})
	}

	runtimeDir := filepath.Join(layout.Root, "runtime")
	if err := secureDirectoryPath(layout.Root, runtimeDir, false); err != nil {
		return nil, err
	}
	runtimeLayer, err := newViewLayer(runtimeDir)
	if err != nil {
		return nil, err
	}
	runGuard, err := paths.NewRunGuardWithRoots(cwd, snapshot.NodeID, readRoots, writeDir, runtimeDir)
	if err != nil {
		return nil, err
	}
	runtimeReadPath, err := filepath.Rel(cwd, runtimeDir)
	if err != nil {
		return nil, err
	}
	return &View{
		projectID: projectID, treeID: snapshot.TreeID, treeDir: treeDir, cwd: cwd, writeDir: writeDir,
		chainPath: []string{}, inputNodeIDs: snapshot.ScopeNodeIDs(), layers: []viewLayer{runtimeLayer},
		sourceLayers: sourceLayers, inputSources: inputSources,
		runGuard: runGuard, runtimeReadPath: filepath.ToSlash(runtimeReadPath),
	}, nil
}

// NewDirectView preserves M1's single physical workspace while sharing the
// same bounded reader implementation used by M2.
func NewDirectView(root, directory string) (*View, error) {
	if strings.TrimSpace(directory) == "" {
		return nil, fmt.Errorf("%w: workspace is required", paths.ErrWorkspaceScopeInvalid)
	}
	layer, err := newViewLayer(directory)
	if err != nil {
		return nil, err
	}
	treeDir, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(root) != "" {
		rootAbs, rootErr := filepath.Abs(root)
		if rootErr != nil {
			return nil, rootErr
		}
		if !pathWithin(rootAbs, treeDir) {
			return nil, fmt.Errorf("%w: workspace is outside root", paths.ErrPathDenied)
		}
		if err := secureDirectoryPath(rootAbs, treeDir, false); err != nil {
			return nil, err
		}
	}
	return &View{treeDir: treeDir, writeDir: treeDir, layers: []viewLayer{layer}, allowAbsolute: true}, nil
}

func newViewLayer(root string) (viewLayer, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return viewLayer{}, err
	}
	if info, statErr := os.Lstat(absolute); statErr == nil {
		if !info.IsDir() {
			return viewLayer{}, fmt.Errorf("%w: overlay layer is not a directory", paths.ErrPathDenied)
		}
		reparse, reparseErr := paths.IsReparsePoint(absolute)
		if reparseErr != nil {
			return viewLayer{}, reparseErr
		}
		if reparse {
			return viewLayer{}, fmt.Errorf("%w: overlay layer is a reparse point/junction", paths.ErrPathDenied)
		}
	} else if !os.IsNotExist(statErr) {
		return viewLayer{}, statErr
	}
	guard, err := paths.NewGuard(absolute, absolute)
	if err != nil {
		return viewLayer{}, err
	}
	return viewLayer{root: absolute, guard: guard}, nil
}

func (v *View) TreeDir() string     { return v.treeDir }
func (v *View) WriteDir() string    { return v.writeDir }
func (v *View) ChainPath() []string { return append([]string(nil), v.chainPath...) }
func (v *View) CWD() string {
	if v.cwd != "" {
		return v.cwd
	}
	return v.treeDir
}
func (v *View) RuntimeReadPath() string { return v.runtimeReadPath }
func (v *View) InputSources() []port.WorkspaceSource {
	return append([]port.WorkspaceSource(nil), v.inputSources...)
}
func (v *View) Resolver() port.ScopedPathResolver {
	if v.runGuard == nil {
		return nil
	}
	return v.runGuard
}

// Scope is empty for the legacy direct view, which is not a run-scoped
// overlay and must not be used to construct M2 path context.
func (v *View) Scope() port.WorkspaceScope {
	if v.projectID == "" {
		return port.WorkspaceScope{}
	}
	treeID := v.treeID
	if treeID == "" && len(v.chainPath) > 0 {
		treeID = v.chainPath[0]
	}
	inputNodeIDs := v.inputNodeIDs
	if len(inputNodeIDs) == 0 {
		inputNodeIDs = v.chainPath
	}
	return port.WorkspaceScope{
		ProjectID: v.projectID, TreeID: treeID, NodeID: v.ScopeNodeID(),
		ChainPath: v.ChainPath(), InputNodeIDs: append([]string(nil), inputNodeIDs...),
	}
}

func (v *View) ScopeNodeID() string {
	if len(v.chainPath) > 0 {
		return v.chainPath[len(v.chainPath)-1]
	}
	if len(v.inputNodeIDs) > 0 {
		return v.inputNodeIDs[len(v.inputNodeIDs)-1]
	}
	return ""
}

func secureDirectoryPath(root, target string, create bool) error {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("%w: root: %v", paths.ErrWorkspaceScopeInvalid, err)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("%w: target: %v", paths.ErrWorkspaceScopeInvalid, err)
	}
	if !pathWithin(rootAbs, targetAbs) {
		return fmt.Errorf("%w: directory escapes application root", paths.ErrPathDenied)
	}
	rootInfo, err := os.Lstat(rootAbs)
	if err != nil {
		return fmt.Errorf("%w: root directory: %v", paths.ErrWorkspaceScopeInvalid, err)
	}
	if !rootInfo.IsDir() {
		return fmt.Errorf("%w: root is not a directory", paths.ErrWorkspaceScopeInvalid)
	}
	rootReparse, err := paths.IsReparsePoint(rootAbs)
	if err != nil {
		return fmt.Errorf("%w: inspect root: %v", paths.ErrWorkspaceScopeInvalid, err)
	}
	if rootReparse {
		return fmt.Errorf("%w: application root is a reparse point", paths.ErrPathDenied)
	}

	rel, err := filepath.Rel(rootAbs, targetAbs)
	if err != nil {
		return fmt.Errorf("%w: relative directory path: %v", paths.ErrWorkspaceScopeInvalid, err)
	}
	if rel == "." {
		return nil
	}
	current := rootAbs
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if !os.IsNotExist(statErr) {
				return fmt.Errorf("%w: inspect %s: %v", paths.ErrWorkspaceScopeInvalid, current, statErr)
			}
			if !create {
				return nil
			}
			if mkdirErr := os.Mkdir(current, 0o755); mkdirErr != nil && !os.IsExist(mkdirErr) {
				return fmt.Errorf("create workspace directory %s: %w", current, mkdirErr)
			}
			info, statErr = os.Lstat(current)
			if statErr != nil {
				return fmt.Errorf("inspect workspace directory %s: %w", current, statErr)
			}
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: path component is not a directory: %s", paths.ErrPathDenied, current)
		}
		reparse, reparseErr := paths.IsReparsePoint(current)
		if reparseErr != nil {
			return fmt.Errorf("%w: inspect workspace directory %s: %v", paths.ErrPathDenied, current, reparseErr)
		}
		if reparse {
			return fmt.Errorf("%w: workspace directory is a reparse point: %s", paths.ErrPathDenied, current)
		}
	}
	return nil
}

func pathWithin(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
