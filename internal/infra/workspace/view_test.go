package workspace

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/port"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

func viewFixture(t *testing.T) (layout valueobject.Layout, layoutProjectID string, view *View) {
	t.Helper()
	layout, project := moverFixtures(t)
	layout.RuntimeDir = filepath.Join(layout.Root, "runtime")

	writeStep(t, project, "n_root", "n_root", "root.txt", "VISIBLE root")
	writeStep(t, project, "n_root", "n_parent", "parent.txt", "VISIBLE parent")
	writeStep(t, project, "n_root", "n_parent", "shared.txt", "HIDDEN parent")
	writeStep(t, project, "n_root", "n_current", "shared.txt", "VISIBLE current")
	writeStep(t, project, "n_root", "n_sibling", "secret.txt", "VISIBLE sibling")
	writeFile(t, filepath.Join(layout.RuntimeDir, "runtime.txt"), "VISIBLE runtime")

	writeStep(t, project, "n_root", "n_root", filepath.Join("docs", "root.md"), "root")
	writeStep(t, project, "n_root", "n_parent", filepath.Join("docs", "parent.md"), "parent")
	writeStep(t, project, "n_root", "n_current", filepath.Join("docs", "current.md"), "current")

	runView, err := NewRunView(layout, "p_test", "n_root", "n_current", []string{"n_root", "n_parent", "n_current"})
	if err != nil {
		t.Fatal(err)
	}
	return layout, "p_test", runView
}

func TestViewReadUsesCurrentAncestorAndRuntimeOrder(t *testing.T) {
	_, _, view := viewFixture(t)
	ctx := context.Background()

	cases := []struct {
		path string
		want string
	}{
		{path: "shared.txt", want: "VISIBLE current"},
		{path: "parent.txt", want: "VISIBLE parent"},
		{path: "root.txt", want: "VISIBLE root"},
		{path: "runtime.txt", want: "VISIBLE runtime"},
	}
	for _, tc := range cases {
		read, err := view.ReadFile(ctx, tc.path, 0)
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v", tc.path, err)
		}
		if string(read.Data) != tc.want || read.Path != tc.path || read.Truncated {
			t.Fatalf("ReadFile(%q) = %+v data=%q", tc.path, read, read.Data)
		}
	}
	if _, err := view.ReadFile(ctx, "secret.txt", 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sibling read error = %v, want not exist", err)
	}
	for _, input := range []string{"../secret.txt", "steps/n_sibling/secret.txt", "runtime/runtime.txt", `C:\Windows\win.ini`} {
		if _, err := view.ReadFile(ctx, input, 0); !errors.Is(err, paths.ErrPathDenied) {
			t.Fatalf("ReadFile(%q) error = %v, want PATH_DENIED", input, err)
		}
	}
}

func TestViewListMergesDirectoriesAndShadowsNames(t *testing.T) {
	_, _, view := viewFixture(t)
	ctx := context.Background()
	listing, err := view.ListDir(ctx, ".", 100)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(listing.Entries))
	for _, entry := range listing.Entries {
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	want := []string{"docs", "parent.txt", "root.txt", "runtime.txt", "shared.txt"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("root entries = %v, want %v", names, want)
	}

	docs, err := view.ListDir(ctx, "docs", 100)
	if err != nil {
		t.Fatal(err)
	}
	docNames := make([]string, 0, len(docs.Entries))
	for _, entry := range docs.Entries {
		docNames = append(docNames, entry.Name)
	}
	sort.Strings(docNames)
	if got, wantDocs := strings.Join(docNames, ","), "current.md,parent.md,root.md"; got != wantDocs {
		t.Fatalf("docs entries = %s, want %s", got, wantDocs)
	}

	limited, err := view.ListDir(ctx, ".", 2)
	if err != nil {
		t.Fatal(err)
	}
	if !limited.Truncated || len(limited.Entries) != 2 {
		t.Fatalf("limited listing = %+v", limited)
	}
}

func TestSnapshotRunViewSeparatesSameNamedFilesAcrossWorkspaceRoots(t *testing.T) {
	layout, projectPaths := moverFixtures(t)
	layout.RuntimeDir = filepath.Join(layout.Root, "runtime")
	writeFile(t, filepath.Join(layout.RuntimeDir, "runtime.txt"), "runtime")
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "多根输入", Revision: 1,
		Nodes: []entity.Node{
			{ID: "n_topic_a", Type: valueobject.NodeTypeTopic, Title: "根 A", WorkspaceRootID: "n_topic_a", Rounds: []entity.Round{}},
			{ID: "n_topic_b", Type: valueobject.NodeTypeTopic, Title: "根 B", WorkspaceRootID: "n_topic_b", Rounds: []entity.Round{}},
			{ID: "n_prompt_a", Type: valueobject.NodeTypePrompt, Title: "任务 A", WorkspaceRootID: "n_topic_a", Params: entity.NodeParams{Instruction: "任务 A"}, Rounds: []entity.Round{}},
			{ID: "n_prompt_b", Type: valueobject.NodeTypePrompt, Title: "任务 B", WorkspaceRootID: "n_topic_b", Params: entity.NodeParams{Instruction: "任务 B"}, Rounds: []entity.Round{}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", WorkspaceRootID: "n_topic_a", Rounds: []entity.Round{}},
		},
		Edges: []entity.Edge{
			{ID: "e_prompt_a", SourceNodeID: "n_prompt_a", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection},
			{ID: "e_prompt_b", SourceNodeID: "n_prompt_b", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection},
		},
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRun("n_agent")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Version != 4 || len(snapshot.InputRoots) != 2 {
		t.Fatalf("multi-root snapshot = %+v", snapshot)
	}
	for _, source := range []struct{ rootID, nodeID, text string }{
		{"n_topic_a", "n_prompt_a", "来自 A"},
		{"n_topic_b", "n_prompt_b", "来自 B"},
	} {
		step, err := projectPaths.StepDir(source.rootID, source.nodeID)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(step, "same.txt"), source.text)
	}

	view, err := NewSnapshotRunView(layout, project.ID, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []struct{ rootID, nodeID, text string }{
		{"n_topic_a", "n_prompt_a", "来自 A"},
		{"n_topic_b", "n_prompt_b", "来自 B"},
	} {
		path := "inputs/" + source.rootID + "/" + source.nodeID + "/same.txt"
		read, err := view.ReadFile(context.Background(), path, 0)
		if err != nil || string(read.Data) != source.text {
			t.Fatalf("ReadFile(%q) = %q err=%v", path, read.Data, err)
		}
	}
	listing, err := view.ListDir(context.Background(), "inputs", 20)
	if err != nil || len(listing.Entries) != 2 {
		t.Fatalf("input namespace listing = %+v err=%v", listing, err)
	}
	rootListing, err := view.ListDir(context.Background(), "inputs/n_topic_a/n_prompt_a", 20)
	if err != nil || len(rootListing.Entries) != 1 || rootListing.Entries[0].Name != "same.txt" {
		t.Fatalf("input root listing = %+v err=%v", rootListing, err)
	}
	sources := view.InputSources()
	if len(sources) != 2 || sources[0].LogicalPath != "inputs/n_topic_a/n_prompt_a/" || sources[1].LogicalPath != "inputs/n_topic_b/n_prompt_b/" {
		t.Fatalf("native read manifest = %+v", sources)
	}
	targetTree, err := projectPaths.TreeDir("n_topic_a")
	if err != nil {
		t.Fatal(err)
	}
	targetStep, err := projectPaths.StepDir("n_topic_a", "n_agent")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(view.CWD()) != filepath.Clean(targetTree) || filepath.Clean(view.WriteDir()) != filepath.Clean(targetStep) {
		t.Fatalf("target workspace cwd=%q write=%q", view.CWD(), view.WriteDir())
	}
	if _, err := view.Resolver().ResolveWrite(filepath.Join("steps", "n_agent", "output.txt")); err != nil {
		t.Fatalf("target workspace write was denied: %v", err)
	}
	if _, err := view.Resolver().ResolveRead(filepath.Join(projectPaths.TreesDir, "n_topic_a", "steps", "n_unselected", "secret.txt")); !errors.Is(err, paths.ErrPathDenied) {
		t.Fatalf("unselected node read error = %v, want PATH_DENIED", err)
	}
	if _, err := view.Resolver().ResolveRead(filepath.Join(projectPaths.TreesDir, "n_topic_b", "steps", "n_prompt_b", "same.txt")); err != nil {
		t.Fatalf("authorized cross-root native read: %v", err)
	}
	if _, err := view.Resolver().ResolveWrite(filepath.Join(projectPaths.TreesDir, "n_topic_b", "steps", "n_prompt_b", "same.txt")); !errors.Is(err, paths.ErrPathDenied) {
		t.Fatalf("cross-root write error = %v, want PATH_DENIED", err)
	}
}

func TestSnapshotRunViewUsesProjectWorkspaceWithoutTopic(t *testing.T) {
	layout, projectPaths := moverFixtures(t)
	layout.RuntimeDir = filepath.Join(layout.Root, "runtime")
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_test", Name: "无 Topic", Revision: 1,
		Nodes: []entity.Node{
			{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "任务", Params: entity.NodeParams{Instruction: "执行任务"}, Rounds: []entity.Round{}},
			{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", Rounds: []entity.Round{}},
		},
		Edges: []entity.Edge{{ID: "e_prompt_agent", SourceNodeID: "n_prompt", TargetNodeID: "n_agent", Kind: entity.EdgeKindConnection}},
	}
	graph, err := entity.NewGraph(&project)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := graph.SnapshotForRun("n_agent")
	if err != nil {
		t.Fatal(err)
	}
	step, err := projectPaths.WorkspaceStepDir("n_prompt")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(step, "input.txt"), "project input")

	view, err := NewSnapshotRunView(layout, project.ID, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	read, err := view.ReadFile(context.Background(), "inputs/project/n_prompt/input.txt", 0)
	if err != nil || string(read.Data) != "project input" || view.Scope().TreeID != "" || len(view.Scope().ChainPath) != 0 {
		t.Fatalf("rootless workspace read=%q scope=%+v err=%v", read.Data, view.Scope(), err)
	}
	listing, err := view.ListDir(context.Background(), ".", 20)
	if err != nil || len(listing.Entries) != 1 || listing.Entries[0].Name != "inputs" {
		t.Fatalf("rootless workspace listing=%+v err=%v", listing, err)
	}
	inputRoot := "inputs/project/n_prompt"
	inputListing, err := view.ListDir(context.Background(), inputRoot, 20)
	if err != nil || len(inputListing.Entries) != 1 || inputListing.Entries[0].Name != "input.txt" {
		t.Fatalf("rootless input listing=%+v err=%v", inputListing, err)
	}
	inputStat, err := view.Stat(context.Background(), inputRoot)
	if err != nil || !inputStat.Dir {
		t.Fatalf("rootless input stat=%+v err=%v", inputStat, err)
	}
	targetStep, err := projectPaths.WorkspaceStepDir("n_agent")
	if err != nil {
		t.Fatal(err)
	}
	sources := view.InputSources()
	if len(sources) != 1 || sources[0].LogicalPath != "inputs/project/n_prompt/" || filepath.Clean(sources[0].PhysicalPath) != filepath.Clean(step) {
		t.Fatalf("rootless native read manifest = %+v", sources)
	}
	if filepath.Clean(view.CWD()) != filepath.Clean(targetStep) || filepath.Clean(view.WriteDir()) != filepath.Clean(targetStep) {
		t.Fatalf("rootless target workspace cwd=%q write=%q, want %q", view.CWD(), view.WriteDir(), targetStep)
	}
	if _, err := view.Resolver().ResolveRead(filepath.Join(step, "input.txt")); err != nil {
		t.Fatalf("rootless input native read was denied: %v", err)
	}
	if _, err := view.Resolver().ResolveWrite(filepath.Join(step, "input.txt")); !errors.Is(err, paths.ErrPathDenied) {
		t.Fatalf("rootless input write error = %v, want PATH_DENIED", err)
	}
	if _, err := view.Resolver().ResolveWrite("result.txt"); err != nil {
		t.Fatalf("target workspace write was denied: %v", err)
	}
}

func TestViewReadTruncatesAndStatUsesVisibleFile(t *testing.T) {
	_, _, view := viewFixture(t)
	ctx := context.Background()
	read, err := view.ReadFile(ctx, "shared.txt", 7)
	if err != nil {
		t.Fatal(err)
	}
	if string(read.Data) != "VISIBLE" || !read.Truncated || read.Size != int64(len("VISIBLE current")) {
		t.Fatalf("read = %+v data=%q", read, read.Data)
	}
	stat, err := view.Stat(ctx, "shared.txt")
	if err != nil {
		t.Fatal(err)
	}
	if stat.Dir || stat.Size != int64(len("VISIBLE current")) || stat.Path != "shared.txt" {
		t.Fatalf("stat = %+v", stat)
	}
}

func TestViewEnforcesReadLimitAndUTF8Boundary(t *testing.T) {
	_, _, view := viewFixture(t)
	content := strings.Repeat("a", int(defaultReadBytes)-1) + "中"
	stepFile := filepath.Join(view.WriteDir(), "large.txt")
	if err := os.WriteFile(stepFile, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	read, err := view.ReadFile(context.Background(), "large.txt", defaultReadBytes*4)
	if err != nil {
		t.Fatal(err)
	}
	if !read.Truncated || int64(len(read.Data)) > defaultReadBytes || !utf8.Valid(read.Data) {
		t.Fatalf("bounded read len=%d truncated=%v utf8=%v", len(read.Data), read.Truncated, utf8.Valid(read.Data))
	}
	if _, err := view.ReadFile(context.Background(), "large.txt", defaultReadBytes); err != nil {
		t.Fatalf("default bounded read: %v", err)
	}
}

func TestViewGrepSkipsBinaryFiles(t *testing.T) {
	_, _, view := viewFixture(t)
	if err := os.WriteFile(filepath.Join(view.WriteDir(), "a-binary.bin"), []byte{0xff, 0xfe, 0xfd}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(view.WriteDir(), "z-match.txt"), []byte("TARGET line"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := view.Grep(context.Background(), "TARGET", ".", port.WorkspaceGrepOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Path != "z-match.txt" {
		t.Fatalf("grep hits = %+v", result.Hits)
	}
	if _, err := view.ReadFile(context.Background(), "a-binary.bin", 0); !errors.Is(err, ErrNotUTF8Text) {
		t.Fatalf("ReadFile(binary) error = %v, want WORKSPACE_NOT_UTF8_TEXT", err)
	}
}

func TestViewCreatesTopicTreeAndPathStepDirectories(t *testing.T) {
	layout, _ := moverFixtures(t)
	view, err := NewRunView(layout, "p_test", "n_root", "n_current", []string{"n_root", "n_parent", "n_current"})
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{
		filepath.Join(layout.ProjectsDir, "p_test", "trees", "n_root"),
		filepath.Join(layout.ProjectsDir, "p_test", "trees", "n_root", "steps", "n_root"),
		filepath.Join(layout.ProjectsDir, "p_test", "trees", "n_root", "steps", "n_parent"),
		view.WriteDir(),
	} {
		info, err := os.Stat(directory)
		if err != nil || !info.IsDir() {
			t.Fatalf("directory %s missing or invalid: %v", directory, err)
		}
	}
}

func TestViewRejectsInvalidLogicalPathsAndRuntimeOverride(t *testing.T) {
	layout, _, view := viewFixture(t)
	for _, input := range []string{"foo:stream", "CON.txt", "trailing.", "nested\\node_modules\\secret", strings.Repeat("x", maxLogicalPathBytes+1)} {
		if _, err := view.ReadFile(context.Background(), input, 0); !errors.Is(err, paths.ErrPathDenied) {
			t.Fatalf("ReadFile(%q) error = %v, want PATH_DENIED", input, err)
		}
	}
	layout.RuntimeDir = filepath.Join(layout.Root, "data", "runtime")
	if _, err := NewRunView(layout, "p_test", "n_root", "n_current", []string{"n_root", "n_current"}); !errors.Is(err, paths.ErrWorkspaceScopeInvalid) {
		t.Fatalf("runtime override error = %v, want WORKSPACE_SCOPE_INVALID", err)
	}
}

func TestDirectViewRetainsAbsoluteWorkspacePaths(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "workspace")
	writeFile(t, filepath.Join(workspaceDir, "notes.md"), "hello")
	view, err := NewDirectView(root, workspaceDir)
	if err != nil {
		t.Fatal(err)
	}
	read, err := view.ReadFile(context.Background(), filepath.Join(workspaceDir, "notes.md"), 0)
	if err != nil || string(read.Data) != "hello" {
		t.Fatalf("absolute read = %q err=%v", read.Data, err)
	}
}

func TestViewGrepUsesMergedLogicalFiles(t *testing.T) {
	_, _, view := viewFixture(t)
	result, err := view.Grep(context.Background(), "VISIBLE|HIDDEN", ".", port.WorkspaceGrepOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string)
	for _, hit := range result.Hits {
		got[hit.Path] = hit.Text
	}
	for _, path := range []string{"shared.txt", "parent.txt", "root.txt", "runtime.txt"} {
		if _, ok := got[path]; !ok {
			t.Fatalf("grep missing %s in %+v", path, result.Hits)
		}
	}
	if strings.Contains(strings.Join(mapValues(got), " "), "HIDDEN parent") {
		t.Fatalf("grep leaked shadowed ancestor: %+v", result.Hits)
	}
	if _, ok := got["secret.txt"]; ok {
		t.Fatalf("grep leaked sibling: %+v", result.Hits)
	}
}

func TestViewRejectsMissingActiveNode(t *testing.T) {
	layout, project := moverFixtures(t)
	if err := os.MkdirAll(filepath.Join(project.TreesDir, "n_root"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRunView(layout, "p_test", "n_root", "", []string{"n_root"}); !errors.Is(err, paths.ErrWorkspaceScopeInvalid) {
		t.Fatalf("NewRunView error = %v, want WORKSPACE_SCOPE_INVALID", err)
	}
}

func TestViewFollowsCrossTreeStepMove(t *testing.T) {
	layout, project := moverFixtures(t)
	layout.RuntimeDir = filepath.Join(layout.Root, "runtime")
	writeStep(t, project, "n_src", "n_src", "root.txt", "source root")
	writeStep(t, project, "n_src", "n_child", "moved.txt", "moved")
	if err := os.MkdirAll(filepath.Join(project.TreesDir, "n_dst", "steps", "n_dst"), 0o755); err != nil {
		t.Fatal(err)
	}
	move, err := NewMover(layout).MoveSubtree(context.Background(), "p_test", "n_src", "n_dst", []string{"n_child"})
	if err != nil {
		t.Fatal(err)
	}

	source, err := NewRunView(layout, "p_test", "n_src", "n_src", []string{"n_src"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.ReadFile(context.Background(), "moved.txt", 0); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source view read error = %v, want not exist", err)
	}
	target, err := NewRunView(layout, "p_test", "n_dst", "n_child", []string{"n_dst", "n_child"})
	if err != nil {
		t.Fatal(err)
	}
	read, err := target.ReadFile(context.Background(), "moved.txt", 0)
	if err != nil || string(read.Data) != "moved" {
		t.Fatalf("target read = %q err=%v", read.Data, err)
	}

	if err := move.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	restored, err := NewRunView(layout, "p_test", "n_src", "n_child", []string{"n_src", "n_child"})
	if err != nil {
		t.Fatal(err)
	}
	read, err = restored.ReadFile(context.Background(), "moved.txt", 0)
	if err != nil || string(read.Data) != "moved" {
		t.Fatalf("restored read = %q err=%v", read.Data, err)
	}
}

func TestViewRejectsJunctionEscape(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("junction test is Windows-only")
	}
	_, _, view := viewFixture(t)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "secret")
	link := filepath.Join(view.WriteDir(), "linked")
	if err := exec.Command("cmd.exe", "/d", "/c", "mklink", "/J", link, outside).Run(); err != nil {
		t.Skipf("cannot create junction: %v", err)
	}
	if _, err := view.ReadFile(context.Background(), "linked/secret.txt", 0); !errors.Is(err, paths.ErrPathDenied) {
		t.Fatalf("junction read error = %v, want PATH_DENIED", err)
	}
}

func mapValues(values map[string]string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}
