package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

func moverFixtures(t *testing.T) (valueobject.Layout, paths.ProjectPaths) {
	t.Helper()
	root := t.TempDir()
	dataDir := filepath.Join(root, "data")
	layout := valueobject.Layout{
		Root:         root,
		DataDir:      dataDir,
		LogsDir:      filepath.Join(root, "logs"),
		WebView2Dir:  filepath.Join(root, "webview2"),
		LogFile:      filepath.Join(root, "logs", "app.log"),
		AgentDir:     filepath.Join(dataDir, "agent"),
		WorkspaceDir: filepath.Join(dataDir, "workspace", "chain_default"),
		SessionsFile: filepath.Join(dataDir, "sessions.json"),
		ConfigFile:   filepath.Join(dataDir, "config.json"),
		ProjectsDir:  filepath.Join(dataDir, "projects"),
		BackupDir:    filepath.Join(dataDir, "backup", "m1"),
	}
	project, err := paths.ResolveProject(layout, "p_test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project.Dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return layout, project
}

func writeStep(t *testing.T, project paths.ProjectPaths, rootID, nodeID, file, content string) string {
	t.Helper()
	step, err := project.StepDir(rootID, nodeID)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(step, file), content)
	return step
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMoverMovesAndRollsBackSubtree(t *testing.T) {
	layout, project := moverFixtures(t)
	childStep := writeStep(t, project, "n_src", "n_child", "a.txt", "a")
	grandStep := writeStep(t, project, "n_src", "n_grand", "b.txt", "b")
	mover := NewMover(layout)
	move, err := mover.MoveSubtree(context.Background(), "p_test", "n_src", "n_dst", []string{"n_child", "n_grand", "n_missing"})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{childStep, grandStep} {
		if _, err := os.Stat(source); !os.IsNotExist(err) {
			t.Fatalf("source step still exists: %s", source)
		}
	}
	childTarget, _ := project.StepDir("n_dst", "n_child")
	data, err := os.ReadFile(filepath.Join(childTarget, "a.txt"))
	if err != nil || string(data) != "a" {
		t.Fatalf("target file = %q err=%v", data, err)
	}
	if err := move.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(childStep, "a.txt"))
	if err != nil || string(data) != "a" {
		t.Fatalf("rolled back file = %q err=%v", data, err)
	}
	childTarget, _ = project.StepDir("n_dst", "n_child")
	if _, err := os.Stat(childTarget); !os.IsNotExist(err) {
		t.Fatalf("target step survived rollback: %s", childTarget)
	}
}

func TestMoverIsNoopWithoutSteps(t *testing.T) {
	layout, _ := moverFixtures(t)
	mover := NewMover(layout)
	move, err := mover.MoveSubtree(context.Background(), "p_test", "n_src", "n_dst", []string{"n_child"})
	if err != nil {
		t.Fatal(err)
	}
	if err := move.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
	move, err = mover.MoveSubtree(context.Background(), "p_test", "n_src", "n_src", []string{"n_child"})
	if err != nil {
		t.Fatal(err)
	}
	if err := move.Rollback(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMoverRejectsExistingTarget(t *testing.T) {
	layout, project := moverFixtures(t)
	sourceStep := writeStep(t, project, "n_src", "n_child", "a.txt", "a")
	targetStep, _ := project.StepDir("n_dst", "n_child")
	if err := os.MkdirAll(targetStep, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewMover(layout).MoveSubtree(context.Background(), "p_test", "n_src", "n_dst", []string{"n_child"}); err == nil {
		t.Fatal("MoveSubtree error = nil, want target exists")
	}
	if _, err := os.Stat(sourceStep); err != nil {
		t.Fatalf("source step changed: %v", err)
	}
}

func TestMoverRejectsInvalidInputs(t *testing.T) {
	layout, project := moverFixtures(t)
	mover := NewMover(layout)
	if _, err := mover.MoveSubtree(context.Background(), "../escape", "n_src", "n_dst", []string{"n_child"}); err == nil {
		t.Fatal("invalid project id accepted")
	}
	if _, err := mover.MoveSubtree(context.Background(), "p_test", "n_src", "../escape", []string{"n_child"}); err == nil {
		t.Fatal("invalid target root accepted")
	}
	targetTree := filepath.Join(project.TreesDir, "n_dst")
	writeFile(t, targetTree, "not a directory")
	if _, err := mover.MoveSubtree(context.Background(), "p_test", "n_src", "n_dst", []string{"n_child"}); err == nil {
		t.Fatal("file target root accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := mover.MoveSubtree(ctx, "p_test", "n_src", "n_dst", []string{"n_child"}); err == nil {
		t.Fatal("canceled context accepted")
	}
}
