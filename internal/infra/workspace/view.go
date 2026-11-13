package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

const (
	defaultReadBytes    = int64(256 * 1024)
	defaultEntries      = 200
	defaultGrepHits     = 200
	defaultGrepFiles    = 5000
	defaultGrepBytes    = int64(512 * 1024)
	maxWalkEntries      = 5000
	maxLogicalPathBytes = 4096
)

var ErrNotUTF8Text = errors.New("WORKSPACE_NOT_UTF8_TEXT")

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

func (v *View) ReadFile(ctx context.Context, input string, maxBytes int64) (port.WorkspaceRead, error) {
	if err := ctx.Err(); err != nil {
		return port.WorkspaceRead{}, err
	}
	clean, err := v.cleanPath(input)
	if err != nil {
		return port.WorkspaceRead{}, err
	}
	physical, info, err := v.lookup(clean)
	if err != nil {
		return port.WorkspaceRead{}, err
	}
	if info.IsDir() {
		return port.WorkspaceRead{}, fmt.Errorf("cannot read a directory: %s", logicalPath(clean))
	}
	if maxBytes <= 0 || maxBytes > defaultReadBytes {
		maxBytes = defaultReadBytes
	}
	file, err := os.Open(physical)
	if err != nil {
		return port.WorkspaceRead{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return port.WorkspaceRead{}, err
	}
	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = validUTF8Prefix(data[:maxBytes])
	}
	if !utf8.Valid(data) {
		return port.WorkspaceRead{}, fmt.Errorf("%w: file is not valid UTF-8 text", ErrNotUTF8Text)
	}
	return port.WorkspaceRead{
		Path:      logicalPath(clean),
		Data:      data,
		Size:      info.Size(),
		Truncated: truncated,
	}, nil
}

func (v *View) ListDir(ctx context.Context, input string, maxEntries int) (port.WorkspaceList, error) {
	if err := ctx.Err(); err != nil {
		return port.WorkspaceList{}, err
	}
	clean, err := v.cleanPath(input)
	if err != nil {
		return port.WorkspaceList{}, err
	}
	if namespace, relative, level, ok := inputNamespace(clean); ok {
		return v.listInputNamespace(ctx, clean, namespace, relative, level, maxEntries)
	}
	var top os.FileInfo
	if clean == "." && len(v.sourceLayers) > 0 {
		top, err = os.Stat(v.CWD())
	} else {
		_, top, err = v.lookup(clean)
	}
	if err != nil {
		return port.WorkspaceList{}, err
	}
	if !top.IsDir() {
		return port.WorkspaceList{}, fmt.Errorf("cannot list a file: %s", logicalPath(clean))
	}

	visible := make(map[string]port.WorkspaceEntry)
	seen := make(map[string]struct{})
	if clean == "." && len(v.sourceLayers) > 0 {
		visible["inputs"] = port.WorkspaceEntry{Name: "inputs", Dir: true}
		seen["inputs"] = struct{}{}
	}
	for _, layer := range v.layers {
		if err := ctx.Err(); err != nil {
			return port.WorkspaceList{}, err
		}
		physical, resolveErr := layer.guard.Resolve(clean)
		if resolveErr != nil {
			return port.WorkspaceList{}, resolveErr
		}
		info, statErr := os.Lstat(physical)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return port.WorkspaceList{}, statErr
		}
		if !info.IsDir() {
			continue
		}
		entries, readErr := os.ReadDir(physical)
		if readErr != nil {
			return port.WorkspaceList{}, readErr
		}
		for _, entry := range entries {
			nameKey := strings.ToLower(entry.Name())
			if _, exists := seen[nameKey]; exists {
				continue
			}
			seen[nameKey] = struct{}{}
			if reservedLogicalName(entry.Name()) {
				continue
			}
			child := filepath.Join(clean, entry.Name())
			if _, childErr := layer.guard.Resolve(child); childErr != nil {
				// A reparse entry remains shadowing but is never exposed.
				continue
			}
			entryInfo, infoErr := entry.Info()
			if infoErr != nil {
				return port.WorkspaceList{}, infoErr
			}
			visible[nameKey] = port.WorkspaceEntry{Name: entry.Name(), Dir: entryInfo.IsDir(), Size: entryInfo.Size()}
		}
	}

	entries := make([]port.WorkspaceEntry, 0, len(visible))
	for _, entry := range visible {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	if maxEntries <= 0 {
		maxEntries = defaultEntries
	} else if maxEntries > maxWalkEntries {
		maxEntries = maxWalkEntries
	}
	result := port.WorkspaceList{Path: logicalPath(clean), Entries: entries}
	if len(result.Entries) > maxEntries {
		result.Entries = result.Entries[:maxEntries]
		result.Truncated = true
	}
	return result, nil
}

func (v *View) Stat(ctx context.Context, input string) (port.WorkspaceStat, error) {
	if err := ctx.Err(); err != nil {
		return port.WorkspaceStat{}, err
	}
	clean, err := v.cleanPath(input)
	if err != nil {
		return port.WorkspaceStat{}, err
	}
	_, info, err := v.lookup(clean)
	if err != nil {
		return port.WorkspaceStat{}, err
	}
	return port.WorkspaceStat{
		Path:    logicalPath(clean),
		Dir:     info.IsDir(),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

func (v *View) Grep(ctx context.Context, expression, input string, options port.WorkspaceGrepOptions) (port.WorkspaceGrep, error) {
	if err := ctx.Err(); err != nil {
		return port.WorkspaceGrep{}, err
	}
	if len(expression) > 256 {
		return port.WorkspaceGrep{}, errors.New("pattern too long")
	}
	pattern, err := regexp.Compile(expression)
	if err != nil {
		return port.WorkspaceGrep{}, fmt.Errorf("invalid pattern: %w", err)
	}
	clean, err := v.cleanPath(input)
	if err != nil {
		return port.WorkspaceGrep{}, err
	}
	if options.MaxHits <= 0 {
		options.MaxHits = defaultGrepHits
	} else if options.MaxHits > defaultGrepHits {
		options.MaxHits = defaultGrepHits
	}
	if options.MaxFiles <= 0 {
		options.MaxFiles = defaultGrepFiles
	} else if options.MaxFiles > defaultGrepFiles {
		options.MaxFiles = defaultGrepFiles
	}
	if options.MaxFileBytes <= 0 || options.MaxFileBytes > defaultGrepBytes {
		options.MaxFileBytes = defaultGrepBytes
	}

	result := port.WorkspaceGrep{Hits: []port.WorkspaceGrepHit{}}
	files := 0
	stop := false
	var walk func(string) error
	walk = func(logical string) error {
		if stop {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		stat, statErr := v.Stat(ctx, logical)
		if statErr != nil {
			return statErr
		}
		if !stat.Dir {
			files++
			if files > options.MaxFiles {
				result.Truncated = true
				stop = true
				return nil
			}
			read, readErr := v.ReadFile(ctx, logical, options.MaxFileBytes)
			if readErr != nil {
				if errors.Is(readErr, ErrNotUTF8Text) {
					return nil
				}
				return readErr
			}
			if read.Truncated {
				result.Truncated = true
			}
			data := validUTF8Prefix(read.Data)
			if !utf8.Valid(data) {
				return nil
			}
			for lineIndex, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
				if pattern.MatchString(line) {
					result.Hits = append(result.Hits, port.WorkspaceGrepHit{
						Path: logicalPath(logical),
						Line: lineIndex + 1,
						Text: trimRunes(strings.TrimSpace(line), 240),
					})
					if len(result.Hits) >= options.MaxHits {
						result.Truncated = true
						stop = true
						return nil
					}
				}
			}
			return nil
		}

		listing, listErr := v.ListDir(ctx, logical, maxWalkEntries)
		if listErr != nil {
			return listErr
		}
		if listing.Truncated {
			result.Truncated = true
		}
		for _, entry := range listing.Entries {
			if err := walk(pathpkg.Join(logicalPath(logical), entry.Name)); err != nil {
				return err
			}
			if stop {
				break
			}
		}
		return nil
	}
	if err := walk(clean); err != nil {
		return port.WorkspaceGrep{}, err
	}
	return result, nil
}

func inputNamespace(clean string) (string, string, int, bool) {
	parts := strings.Split(filepath.ToSlash(clean), "/")
	if len(parts) == 0 || parts[0] != "inputs" {
		return "", "", 0, false
	}
	if len(parts) == 1 {
		return "", "", 1, true
	}
	if len(parts) == 2 {
		return "", parts[1], 2, true
	}
	return strings.Join(parts[:3], "/"), strings.Join(parts[3:], "/"), 3, true
}

func (v *View) listInputNamespace(ctx context.Context, clean, namespace, relative string, level, maxEntries int) (port.WorkspaceList, error) {
	if err := ctx.Err(); err != nil {
		return port.WorkspaceList{}, err
	}
	if level < 3 {
		seen := make(map[string]struct{})
		entries := make([]port.WorkspaceEntry, 0)
		for key := range v.sourceLayers {
			parts := strings.Split(key, "/")
			if level == 1 {
				if _, exists := seen[parts[1]]; !exists {
					seen[parts[1]] = struct{}{}
					entries = append(entries, port.WorkspaceEntry{Name: parts[1], Dir: true})
				}
			} else if parts[1] == relative {
				if _, exists := seen[parts[2]]; !exists {
					seen[parts[2]] = struct{}{}
					entries = append(entries, port.WorkspaceEntry{Name: parts[2], Dir: true})
				}
			}
		}
		sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name) })
		if maxEntries <= 0 {
			maxEntries = defaultEntries
		} else if maxEntries > maxWalkEntries {
			maxEntries = maxWalkEntries
		}
		result := port.WorkspaceList{Path: logicalPath(clean), Entries: entries}
		if len(result.Entries) > maxEntries {
			result.Entries = result.Entries[:maxEntries]
			result.Truncated = true
		}
		return result, nil
	}
	layer, exists := v.sourceLayers[namespace]
	if !exists {
		return port.WorkspaceList{}, fmt.Errorf("%w: %s", os.ErrNotExist, logicalPath(clean))
	}
	if relative == "" {
		relative = "."
	}
	physical, err := layer.guard.Resolve(relative)
	if err != nil {
		return port.WorkspaceList{}, err
	}
	info, err := os.Lstat(physical)
	if err != nil {
		return port.WorkspaceList{}, err
	}
	if !info.IsDir() {
		return port.WorkspaceList{}, fmt.Errorf("cannot list a file: %s", logicalPath(clean))
	}
	dirEntries, err := os.ReadDir(physical)
	if err != nil {
		return port.WorkspaceList{}, err
	}
	entries := make([]port.WorkspaceEntry, 0, len(dirEntries))
	seen := make(map[string]struct{})
	for _, entry := range dirEntries {
		nameKey := strings.ToLower(entry.Name())
		if _, duplicate := seen[nameKey]; duplicate || reservedLogicalName(entry.Name()) {
			continue
		}
		seen[nameKey] = struct{}{}
		child := filepath.Join(relative, entry.Name())
		if _, err := layer.guard.Resolve(child); err != nil {
			continue
		}
		entryInfo, err := entry.Info()
		if err != nil {
			return port.WorkspaceList{}, err
		}
		entries = append(entries, port.WorkspaceEntry{Name: entry.Name(), Dir: entryInfo.IsDir(), Size: entryInfo.Size()})
	}
	sort.Slice(entries, func(i, j int) bool { return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name) })
	if maxEntries <= 0 {
		maxEntries = defaultEntries
	} else if maxEntries > maxWalkEntries {
		maxEntries = maxWalkEntries
	}
	result := port.WorkspaceList{Path: logicalPath(clean), Entries: entries}
	if len(result.Entries) > maxEntries {
		result.Entries = result.Entries[:maxEntries]
		result.Truncated = true
	}
	return result, nil
}

func (v *View) lookup(clean string) (string, os.FileInfo, error) {
	if namespace, relative, level, ok := inputNamespace(clean); ok {
		if level < 3 {
			info, err := os.Stat(v.treeDir)
			return v.treeDir, info, err
		}
		layer, exists := v.sourceLayers[namespace]
		if !exists {
			return "", nil, fmt.Errorf("%w: %s", os.ErrNotExist, logicalPath(clean))
		}
		if relative == "" {
			relative = "."
		}
		physical, err := layer.guard.Resolve(relative)
		if err != nil {
			return "", nil, err
		}
		info, err := os.Lstat(physical)
		return physical, info, err
	}
	for _, layer := range v.layers {
		physical, err := layer.guard.Resolve(clean)
		if err != nil {
			return "", nil, err
		}
		info, err := os.Lstat(physical)
		if err == nil {
			return physical, info, nil
		}
		if !os.IsNotExist(err) {
			return "", nil, err
		}
	}
	return "", nil, fmt.Errorf("%w: %s", os.ErrNotExist, logicalPath(clean))
}

func (v *View) cleanPath(input string) (string, error) {
	if !v.allowAbsolute || (!filepath.IsAbs(input) && filepath.VolumeName(input) == "" && !strings.HasPrefix(input, `\\`) && !strings.HasPrefix(input, "/")) {
		return cleanLogicalPath(input)
	}
	for _, layer := range v.layers {
		resolved, err := layer.guard.Resolve(input)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(layer.root, resolved)
		if err != nil {
			return "", fmt.Errorf("%w: cannot map workspace path", paths.ErrPathDenied)
		}
		return cleanLogicalPath(relative)
	}
	return "", fmt.Errorf("%w: absolute path is outside workspace", paths.ErrPathDenied)
}

func cleanLogicalPath(input string) (string, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return "", fmt.Errorf("%w: empty path", paths.ErrPathDenied)
	}
	if filepath.IsAbs(raw) || filepath.VolumeName(raw) != "" || strings.HasPrefix(raw, `\`) || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("%w: logical paths must be relative", paths.ErrPathDenied)
	}
	if len(raw) > maxLogicalPathBytes {
		return "", fmt.Errorf("%w: logical path is too long", paths.ErrPathDenied)
	}
	clean := filepath.Clean(raw)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: path escapes workspace", paths.ErrPathDenied)
	}
	if clean == "." {
		return clean, nil
	}
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if reservedLogicalName(part) || !validLogicalPathPart(part) {
			return "", fmt.Errorf("%w: protected or invalid logical path component %s", paths.ErrPathDenied, part)
		}
	}
	return clean, nil
}

func validLogicalPathPart(part string) bool {
	if part == "" || strings.ContainsAny(part, `<>:"|?*`) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
		return false
	}
	for _, r := range part {
		if r < 0x20 {
			return false
		}
	}
	base := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	default:
		return true
	}
}

func reservedLogicalName(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "webview2", "runtime", "artifacts", "steps":
		return true
	default:
		return false
	}
}

func logicalPath(path string) string {
	if path == "" || path == "." {
		return "."
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func validUTF8Prefix(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	for trim := 1; trim <= 3 && trim < len(data); trim++ {
		prefix := data[:len(data)-trim]
		if utf8.Valid(prefix) {
			return prefix
		}
	}
	return nil
}

func trimRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	count := 0
	for index := range text {
		if count == limit {
			return text[:index]
		}
		count++
	}
	return text
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
