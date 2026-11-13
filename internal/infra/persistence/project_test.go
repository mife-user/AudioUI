package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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

func TestProjectStorePersistsNodeLayoutAndCollapsedState(t *testing.T) {
	ctx := context.Background()
	store := NewProjectStore(testLayout(t.TempDir()))
	project := sampleProject("p_layout")
	if err := store.Create(ctx, project); err != nil {
		t.Fatal(err)
	}
	project.Nodes[0].Position = entity.Position{X: 37, Y: 48}
	project.Nodes[0].Size = entity.NodeSize{Width: 345, Height: 275}
	project.Nodes[0].Collapsed = true
	project.Revision++
	if err := store.Save(ctx, project); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, project.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Nodes[0].Position != project.Nodes[0].Position || loaded.Nodes[0].Size != project.Nodes[0].Size || !loaded.Nodes[0].Collapsed {
		t.Fatalf("loaded node layout = position %+v size %+v collapsed=%t", loaded.Nodes[0].Position, loaded.Nodes[0].Size, loaded.Nodes[0].Collapsed)
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
	project.Nodes[0].Rounds = []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:x", RunID: "r_topics"}}
	project.Nodes = append(project.Nodes,
		entity.Node{
			ID: "n_candidate", Type: valueobject.NodeTypeCandidate, Title: "候选", Size: entity.DefaultNodeSize(valueobject.NodeTypeCandidate),
			CandidateSource: &entity.CandidateSource{TopicNodeID: "n_root", Round: 1, ArtifactID: "sha256:x", Index: 0, RunID: "r_topics"},
			Rounds:          []entity.Round{},
		},
		entity.Node{
			ID: "n_child", Type: valueobject.NodeTypePrompt, Title: "提示词",
			Params: entity.NodeParams{Instruction: "写脚本"},
			Size:   entity.DefaultNodeSize(valueobject.NodeTypePrompt),
			Rounds: []entity.Round{},
		},
	)
	linkNodes(&project, "n_root", "n_candidate")
	linkNodes(&project, "n_candidate", "n_child")
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
	for _, forbidden := range []string{"chains", "parentCandidate"} {
		if _, ok := raw[forbidden]; ok {
			t.Fatalf("project.json contains derived field %q", forbidden)
		}
	}
	persistedNodes, ok := raw["nodes"].([]any)
	if !ok || len(persistedNodes) != 3 {
		t.Fatalf("persisted nodes = %#v", raw["nodes"])
	}
	persistedEdges, ok := raw["edges"].([]any)
	if !ok || len(persistedEdges) != 2 {
		t.Fatalf("persisted edges = %#v", raw["edges"])
	}
	persistedSize, ok := persistedNodes[0].(map[string]any)["size"].(map[string]any)
	if !ok || persistedSize["width"] != float64(entity.DefaultNodeSize(valueobject.NodeTypeTopic).Width) || persistedSize["height"] != float64(entity.DefaultNodeSize(valueobject.NodeTypeTopic).Height) {
		t.Fatalf("persisted topic size = %#v", persistedNodes[0])
	}
	loaded, err := store.Load(context.Background(), "p_test")
	if err != nil {
		t.Fatal(err)
	}
	child := loaded.Nodes[2]
	if child.Parent != "" || child.ParentCandidate != nil {
		t.Fatalf("child = %+v", child)
	}
	if len(loaded.Edges) != 2 || loaded.Edges[1].SourceNodeID != "n_candidate" || loaded.Edges[1].TargetNodeID != "n_child" {
		t.Fatalf("loaded edges = %+v", loaded.Edges)
	}
}

func TestProjectStorePersistsMultipleUpstreamEdgesAndReopens(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewProjectStore(layout)
	project := sampleProject("p_test")
	project.Nodes[0].Rounds = []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: "sha256:topics", RunID: "r_topics"}}
	project.Nodes = append(project.Nodes,
		entity.Node{ID: "n_candidate", Type: valueobject.NodeTypeCandidate, Title: "候选", Size: entity.DefaultNodeSize(valueobject.NodeTypeCandidate), CandidateSource: &entity.CandidateSource{TopicNodeID: "n_root", Round: 1, ArtifactID: "sha256:topics", Index: 0, RunID: "r_topics"}, Rounds: []entity.Round{}},
		entity.Node{ID: "n_prompt", Type: valueobject.NodeTypePrompt, Title: "任务", Size: entity.DefaultNodeSize(valueobject.NodeTypePrompt), Rounds: []entity.Round{}},
		entity.Node{ID: "n_agent", Type: valueobject.NodeTypeAgent, Title: "Agent", Size: entity.DefaultNodeSize(valueobject.NodeTypeAgent), Rounds: []entity.Round{}},
		entity.Node{ID: "n_script", Type: valueobject.NodeTypeScript, Title: "脚本", Size: entity.DefaultNodeSize(valueobject.NodeTypeScript), Rounds: []entity.Round{}},
	)
	linkNodes(&project, "n_root", "n_candidate")
	linkNodes(&project, "n_candidate", "n_prompt")
	linkNodes(&project, "n_prompt", "n_agent")
	linkNodes(&project, "n_script", "n_agent")
	if err := store.Create(context.Background(), project); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(context.Background(), "p_test")
	if err != nil {
		t.Fatal(err)
	}
	graph, err := entity.NewGraph(&loaded)
	if err != nil {
		t.Fatal(err)
	}
	if parents := graph.Parents("n_agent"); len(parents) != 2 || parents[0] != "n_prompt" || parents[1] != "n_script" {
		t.Fatalf("reopened parents = %v", parents)
	}
	if edge, ok := graph.EdgeBetween("n_script", "n_prompt"); ok {
		t.Fatalf("unexpected edge %+v", edge)
	}
}

func TestProjectStoreRejectsInvalidNodeSizes(t *testing.T) {
	invalidSizes := []entity.NodeSize{
		{},
		{Width: math.NaN(), Height: 200},
		{Width: 200, Height: math.Inf(1)},
		{Width: -1, Height: 200},
		{Width: entity.MinNodeWidth - 1, Height: entity.MinNodeHeight},
		{Width: entity.MaxNodeWidth + 1, Height: entity.MaxNodeHeight},
		{Width: entity.MinNodeWidth, Height: entity.MinNodeHeight - 1},
		{Width: entity.MaxNodeWidth, Height: entity.MaxNodeHeight + 1},
	}
	for _, size := range invalidSizes {
		t.Run(fmt.Sprintf("%v-%v", size.Width, size.Height), func(t *testing.T) {
			project := sampleProject("p_invalid")
			project.Nodes[0].Size = size
			if err := NewProjectStore(testLayout(t.TempDir())).Create(context.Background(), project); !errors.Is(err, ErrInvalidProject) {
				t.Fatalf("Create() error = %v, want ErrInvalidProject", err)
			}
		})
	}

	layout := testLayout(t.TempDir())
	project := sampleProject("p_missing_size")
	project.Nodes[0].Size = entity.NodeSize{}
	writeJSONFileForTest(t, filepath.Join(layout.ProjectsDir, project.ID, "project.json"), project)
	if _, err := NewProjectStore(layout).Load(context.Background(), project.ID); !errors.Is(err, ErrCorruptProject) {
		t.Fatalf("Load() error = %v, want ErrCorruptProject", err)
	}
}

func TestProjectStoreRejectsPreviousLayoutVersion(t *testing.T) {
	layout := testLayout(t.TempDir())
	project := sampleProject("p_previous")
	project.Version = 2
	writeJSONFileForTest(t, filepath.Join(layout.ProjectsDir, project.ID, "project.json"), project)
	if _, err := NewProjectStore(layout).Load(context.Background(), project.ID); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("Load() error = %v, want ErrUnsupportedVersion", err)
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

func TestProjectStoreListsUnsupportedProjectsWithoutBlockingValidOnes(t *testing.T) {
	layout := testLayout(t.TempDir())
	store := NewProjectStore(layout)
	if err := store.Create(context.Background(), sampleProject("p_valid")); err != nil {
		t.Fatal(err)
	}
	old := sampleProject("p_old")
	old.Version = 1
	writeJSONFileForTest(t, filepath.Join(layout.ProjectsDir, "p_old", "project.json"), old)

	summaries, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("summaries = %+v", summaries)
	}
	byID := make(map[string]entity.ProjectSummary, len(summaries))
	for _, summary := range summaries {
		byID[summary.ID] = summary
	}
	if summary := byID["p_old"]; summary.Version != 1 || summary.ErrorCode != ErrUnsupportedVersion.Error() {
		t.Fatalf("unsupported summary = %+v", summary)
	}
	if summary := byID["p_valid"]; summary.Version != valueobject.ProjectFileVersion || summary.ErrorCode != "" {
		t.Fatalf("valid summary = %+v", summary)
	}
	if _, err := store.Load(context.Background(), "p_old"); !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("Load old project error = %v, want UNSUPPORTED_VERSION", err)
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
			entry.Name() != "pending" && entry.Name() != "artifacts" && entry.Name() != "runs" && entry.Name() != "trees" && entry.Name() != "workspace" {
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
