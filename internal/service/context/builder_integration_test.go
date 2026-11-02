package context_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"audio-ui/internal/domain/entity"
	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
	"audio-ui/internal/infra/persistence"
	"audio-ui/internal/infra/workspace"
	contextservice "audio-ui/internal/service/context"
)

func TestRealOverlayManifestReferencesFilesWithoutTheirContents(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	layout := valueobject.Layout{Root: root, ProjectsDir: filepath.Join(root, "data", "projects"), RuntimeDir: filepath.Join(root, "runtime")}
	projectPaths, err := paths.ResolveProject(layout, "p_test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectPaths.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := persistence.NewArtifactStore(projectPaths.Dir)
	artifact, err := store.Put(ctx, []byte(`{"topics":[{"title":"所选选题","angle":"角度","hook":"开头"},{"title":"未选选题"}]}`), entity.ArtifactMeta{Kind: valueobject.OutputKindTopics, NodeID: "n_topic", CreatedAt: "固定时间"})
	if err != nil {
		t.Fatal(err)
	}
	zero, one := 0, 1
	project := entity.Project{ID: "p_test", Nodes: []entity.Node{
		{ID: "n_topic", Type: valueobject.NodeTypeTopic, Title: "选题", Rounds: []entity.Round{{N: 1, Kind: valueobject.OutputKindTopics, ArtifactID: artifact.ID}}},
		{ID: "n_target", Type: valueobject.NodeTypePrompt, Parent: "n_topic", ParentCandidate: &zero, Params: entity.NodeParams{Instruction: "写脚本", OutputKind: valueobject.OutputKindScript}},
		{ID: "n_sibling", Type: valueobject.NodeTypePrompt, Parent: "n_topic", ParentCandidate: &one},
	}}
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rootStep, _ := projectPaths.StepDir("n_topic", "n_topic")
	targetStep, _ := projectPaths.StepDir("n_topic", "n_target")
	siblingStep, _ := projectPaths.StepDir("n_topic", "n_sibling")
	write(filepath.Join(rootStep, "shared.txt"), []byte("ROOT_CONTENT_SECRET"))
	write(filepath.Join(rootStep, "image.bin"), append([]byte("BINARY_CONTENT_SECRET"), 0xff, 0x00))
	write(filepath.Join(targetStep, "shared.txt"), []byte("TARGET_CONTENT_SECRET"))
	write(filepath.Join(siblingStep, "sibling-only.txt"), []byte("SIBLING_CONTENT_SECRET"))
	write(filepath.Join(layout.RuntimeDir, "theme.css"), []byte("RUNTIME_CONTENT_SECRET"))
	view, err := workspace.NewRunView(layout, "p_test", "n_topic", "n_target", []string{"n_topic", "n_target"})
	if err != nil {
		t.Fatal(err)
	}
	scope := view.Scope()
	scope.ChainPath[0] = "n_modified"
	if view.Scope().ChainPath[0] != "n_topic" {
		t.Fatal("workspace scope exposed its internal path")
	}
	builder := contextservice.NewBuilder(persistence.NewArtifactStoreProvider(layout))
	input := contextservice.Input{Project: project, NodeID: "n_target", Workspace: view}
	result, err := builder.Build(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"所选选题", "shared.txt", "image.bin", "theme.css"} {
		if !strings.Contains(result.InitialPrompt, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, forbidden := range []string{"未选选题", "ROOT_CONTENT_SECRET", "TARGET_CONTENT_SECRET", "BINARY_CONTENT_SECRET", "RUNTIME_CONTENT_SECRET", "sibling-only.txt", "SIBLING_CONTENT_SECRET"} {
		if strings.Contains(result.InitialPrompt, forbidden) {
			t.Errorf("unexpected content %q", forbidden)
		}
	}
	var prompt struct {
		Context struct {
			Workspace struct {
				Files []contextservice.WorkspaceFile `json:"files"`
			} `json:"workspace"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(result.InitialPrompt), &prompt); err != nil {
		t.Fatal(err)
	}
	for _, file := range prompt.Context.Workspace.Files {
		if file.Path == "shared.txt" && file.Size != int64(len("TARGET_CONTENT_SECRET")) {
			t.Fatalf("overlay used the wrong file size: %+v", file)
		}
	}
	siblingView, err := workspace.NewRunView(layout, "p_test", "n_topic", "n_sibling", []string{"n_topic", "n_sibling"})
	if err != nil {
		t.Fatal(err)
	}
	input.Workspace = siblingView
	if _, err := builder.Build(ctx, input); !errors.Is(err, contextservice.ErrWorkspaceScope) {
		t.Fatalf("wrong-path workspace was accepted: %v", err)
	}
}
