package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProjectLayout(t *testing.T) {
	layout := layoutFor(t.TempDir())
	project, err := ResolveProject(layout, "p_test")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(layout.ProjectsDir, "p_test")
	for name, got := range map[string]string{
		"dir":        project.Dir,
		"project":    project.ProjectFile,
		"pending":    project.PendingFile,
		"pendingDir": project.PendingDir,
		"artifacts":  project.ArtifactsDir,
		"runs":       project.RunsFile,
		"runsDir":    project.RunsDir,
		"trees":      project.TreesDir,
	} {
		var want string
		switch name {
		case "dir":
			want = dir
		case "project":
			want = filepath.Join(dir, "project.json")
		case "pending":
			want = filepath.Join(dir, "pending.json")
		case "pendingDir":
			want = filepath.Join(dir, "pending")
		case "artifacts":
			want = filepath.Join(dir, "artifacts")
		case "runs":
			want = filepath.Join(dir, "runs.json")
		case "runsDir":
			want = filepath.Join(dir, "runs")
		case "trees":
			want = filepath.Join(dir, "trees")
		}
		if got != want {
			t.Fatalf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestResolveProjectRejectsUnsafeID(t *testing.T) {
	layout := layoutFor(t.TempDir())
	for _, id := range []string{"", "..", "p/one", `p\one`, "P_ONE"} {
		if _, err := ResolveProject(layout, id); err == nil {
			t.Fatalf("ResolveProject(%q) error = nil", id)
		}
	}
}

func TestProjectStepDir(t *testing.T) {
	layout := layoutFor(t.TempDir())
	project, err := ResolveProject(layout, "p_test")
	if err != nil {
		t.Fatal(err)
	}
	step, err := project.StepDir("n_root", "n_child")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(project.TreesDir, "n_root", "steps", "n_child")
	if step != want {
		t.Fatalf("step = %q, want %q", step, want)
	}
	if _, err := project.StepDir("../root", "n_child"); err == nil {
		t.Fatal("StepDir accepted unsafe root id")
	}
}

func TestEnsureProjectDirs(t *testing.T) {
	layout := layoutFor(t.TempDir())
	project, err := ResolveProject(layout, "p_test")
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureProjectDirs(project); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{project.Dir, project.PendingDir, project.ArtifactsDir, project.RunsDir, project.TreesDir} {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Fatalf("directory %s missing: %v", dir, err)
		}
	}
}
