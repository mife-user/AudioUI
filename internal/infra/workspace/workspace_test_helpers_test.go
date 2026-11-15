package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/valueobject"
	"audio-ui/internal/infra/paths"
)

func workspaceFixtures(t *testing.T) (valueobject.Layout, paths.ProjectPaths) {
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
