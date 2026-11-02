package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
)

func TestProjectStoreCreateLoadList(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewProjectStore(layout)
	project := sampleProject("p_test")
	if err := store.Create(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), "p_test")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != project.Name || loaded.ActiveNodeID != "n_root" || len(loaded.Nodes) != 1 {
		t.Fatalf("loaded = %+v", loaded)
	}
	summaries, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].ID != "p_test" {
		t.Fatalf("summaries = %+v", summaries)
	}
	projectPaths := filepath.Join(layout.ProjectsDir, "p_test")
	for _, path := range []string{
		filepath.Join(projectPaths, "project.json"),
		filepath.Join(projectPaths, "pending.json"),
		filepath.Join(projectPaths, "runs.json"),
		filepath.Join(projectPaths, "pending"),
		filepath.Join(projectPaths, "artifacts"),
		filepath.Join(projectPaths, "runs"),
		filepath.Join(projectPaths, "trees"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
}

func TestProjectStoreNewProjectIsEmpty(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewProjectStore(layout)
	project := entity.Project{
		Version: valueobject.ProjectFileVersion, ID: "p_empty", Name: "空工程",
		Revision: 1, Nodes: []entity.Node{},
	}
	if err := store.Create(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), "p_empty")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Nodes == nil || len(loaded.Nodes) != 0 {
		t.Fatalf("nodes = %#v", loaded.Nodes)
	}
}

func TestProjectStorePersistedSchemaHasNoDerivedEdgesOrChains(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewProjectStore(layout)
	project := sampleProject("p_test")
	zero := 0
	project.Nodes = append(project.Nodes, entity.Node{
		ID: "n_child", Type: valueobject.NodeTypePrompt, Title: "提示词",
		Parent: "n_root", ParentCandidate: &zero,
		Params: entity.NodeParams{Instruction: "写脚本", OutputKind: valueobject.OutputKindScript},
		Rounds: []entity.Round{}, Locked: true,
	})
	if err := store.Create(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(layout.ProjectsDir, "p_test", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"chains", "edges"} {
		if _, ok := raw[forbidden]; ok {
			t.Fatalf("project.json contains derived field %q", forbidden)
		}
	}
	loaded, err := store.Load(context.Background(), "p_test")
	if err != nil {
		t.Fatal(err)
	}
	child := loaded.Nodes[1]
	if child.ParentCandidate == nil || *child.ParentCandidate != 0 {
		t.Fatalf("parentCandidate = %v, want pointer to 0", child.ParentCandidate)
	}
}

func TestProjectStoreCreateExistingFails(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewProjectStore(layout)
	project := sampleProject("p_test")
	if err := store.Create(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), project); !errors.Is(err, ErrProjectExists) {
		t.Fatalf("error = %v, want ErrProjectExists", err)
	}
}

func TestProjectStoreLoadMissing(t *testing.T) {
	store := NewProjectStore(testLayout(t.TempDir()))
	if _, err := store.Load(context.Background(), "p_missing"); !errors.Is(err, ErrProjectNotFound) {
		t.Fatalf("error = %v, want ErrProjectNotFound", err)
	}
}

func TestProjectStoreCorruptDoesNotRebuild(t *testing.T) {
	layout := testLayout(t.TempDir())
	projectPaths := filepath.Join(layout.ProjectsDir, "p_test")
	writeTestFile(t, filepath.Join(projectPaths, "project.json"), "{")
	store := NewProjectStore(layout)
	if _, err := store.Load(context.Background(), "p_test"); !errors.Is(err, ErrCorruptProject) {
		t.Fatalf("error = %v, want ErrCorruptProject", err)
	}
	data, err := os.ReadFile(filepath.Join(projectPaths, "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{" {
		t.Fatalf("corrupt project was rewritten: %q", data)
	}
}

func TestProjectStoreUnsupportedVersion(t *testing.T) {
	layout := testLayout(t.TempDir())
	project := sampleProject("p_test")
	project.Version = 99
	writeJSONFileForTest(t, filepath.Join(layout.ProjectsDir, "p_test", "project.json"), project)
	if _, err := NewProjectStore(layout).Load(context.Background(), "p_test"); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("error = %v, want ErrUnsupportedVersion", err)
	}
}

func TestProjectStoreRejectsIDMismatch(t *testing.T) {
	layout := testLayout(t.TempDir())
	project := sampleProject("p_other")
	writeJSONFileForTest(t, filepath.Join(layout.ProjectsDir, "p_test", "project.json"), project)
	if _, err := NewProjectStore(layout).Load(context.Background(), "p_test"); !errors.Is(err, ErrCorruptProject) {
		t.Fatalf("error = %v, want ErrCorruptProject", err)
	}
}

func TestProjectStoreRejectsDuplicateNodeIDs(t *testing.T) {
	layout := testLayout(t.TempDir())
	project := sampleProject("p_test")
	project.Nodes = append(project.Nodes, project.Nodes[0])
	writeJSONFileForTest(t, filepath.Join(layout.ProjectsDir, "p_test", "project.json"), project)
	if _, err := NewProjectStore(layout).Load(context.Background(), "p_test"); !errors.Is(err, ErrCorruptProject) {
		t.Fatalf("error = %v, want ErrCorruptProject", err)
	}
}

func TestProjectStoreSaveReplacesAtomically(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewProjectStore(layout)
	project := sampleProject("p_test")
	if err := store.Create(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	project.Name = "改名"
	project.Revision = 2
	if err := store.Save(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), "p_test")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "改名" || loaded.Revision != 2 {
		t.Fatalf("loaded = %+v", loaded)
	}
	entries, err := os.ReadDir(filepath.Join(layout.ProjectsDir, "p_test"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "project.json" && entry.Name() != "pending.json" && entry.Name() != "runs.json" &&
			entry.Name() != "pending" && entry.Name() != "artifacts" && entry.Name() != "runs" && entry.Name() != "trees" {
			t.Fatalf("unexpected entry %q", entry.Name())
		}
	}
}

func TestProjectStoreListSortsByName(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewProjectStore(layout)
	first := sampleProject("p_a")
	first.Name = "工程 B"
	second := sampleProject("p_b")
	second.Name = "工程 A"
	if err := store.Create(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.Create(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	summaries, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 || summaries[0].Name != "工程 A" || summaries[1].Name != "工程 B" {
		t.Fatalf("summaries = %+v", summaries)
	}
}

func TestProjectStoreExists(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewProjectStore(layout)
	if exists, err := store.Exists(context.Background(), "p_test"); err != nil || exists {
		t.Fatalf("exists = %v err = %v", exists, err)
	}
	if err := store.Create(context.Background(), sampleProject("p_test")); err != nil {
		t.Fatal(err)
	}
	if exists, err := store.Exists(context.Background(), "p_test"); err != nil || !exists {
		t.Fatalf("exists = %v err = %v", exists, err)
	}
}

func writeJSONFileForTest(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, path, string(data))
}
