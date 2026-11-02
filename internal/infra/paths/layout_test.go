package paths

import (
	"os"
	"path/filepath"
	"testing"

	"audio-ui/internal/domain/valueobject"
)

func TestResolveFromExecutable(t *testing.T) {
	root := t.TempDir()
	exe := filepath.Join(root, "audio-ui.exe")

	resolver := NewResolverWith(
		func() (string, error) { return exe, nil },
		func(string) string { return "" },
	)
	layout, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}

	if layout.Root != root {
		t.Fatalf("Root = %q, want %q", layout.Root, root)
	}
	if layout.DataDir != filepath.Join(root, "data") {
		t.Fatalf("DataDir = %q", layout.DataDir)
	}
	if layout.LogsDir != filepath.Join(root, "logs") {
		t.Fatalf("LogsDir = %q", layout.LogsDir)
	}
	if layout.WebView2Dir != filepath.Join(root, "webview2") {
		t.Fatalf("WebView2Dir = %q", layout.WebView2Dir)
	}
	if layout.LogFile != filepath.Join(root, "logs", "app.log") {
		t.Fatalf("LogFile = %q", layout.LogFile)
	}
	if layout.ConfigFile != filepath.Join(root, "data", "config.json") {
		t.Fatalf("ConfigFile = %q", layout.ConfigFile)
	}
	if layout.ProjectsDir != filepath.Join(root, "data", "projects") {
		t.Fatalf("ProjectsDir = %q", layout.ProjectsDir)
	}
	if layout.RuntimeDir != filepath.Join(root, "runtime") {
		t.Fatalf("RuntimeDir = %q", layout.RuntimeDir)
	}
	if layout.BackupDir != filepath.Join(root, "data", "backup", "m1") {
		t.Fatalf("BackupDir = %q", layout.BackupDir)
	}
	if layout.RootSource != "exe" || !layout.Portable {
		t.Fatalf("source = %q portable = %v", layout.RootSource, layout.Portable)
	}
}

func TestResolveFromEnvironmentOverride(t *testing.T) {
	root := t.TempDir()
	resolver := NewResolverWith(
		func() (string, error) { return filepath.Join(root, "ignored.exe"), nil },
		func(key string) string {
			if key == homeEnv {
				return root
			}
			return ""
		},
	)
	layout, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if layout.Root != root {
		t.Fatalf("Root = %q, want %q", layout.Root, root)
	}
	if layout.RootSource != "env" || layout.Portable {
		t.Fatalf("source = %q portable = %v", layout.RootSource, layout.Portable)
	}
}

func TestResolveMissingRoot(t *testing.T) {
	resolver := NewResolverWith(
		func() (string, error) { return "", nil },
		func(string) string { return filepath.Join(t.TempDir(), "missing") },
	)
	if _, err := resolver.Resolve(); err == nil {
		t.Fatal("Resolve() error = nil, want missing directory error")
	}
}

func TestEnsureCreatesWritableDirectories(t *testing.T) {
	root := t.TempDir()
	layout := layoutFor(root)
	if err := Ensure(layout); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}
	for _, dir := range []string{
		layout.DataDir, layout.LogsDir, layout.WebView2Dir,
		layout.AgentDir, layout.WorkspaceDir, layout.ProjectsDir, layout.BackupDir,
	} {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			t.Fatalf("directory %s missing: %v", dir, err)
		}
	}
}

func TestEnsureRejectsFileInPlaceOfDirectory(t *testing.T) {
	root := t.TempDir()
	layout := layoutFor(root)
	if err := os.WriteFile(layout.DataDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(layout); err == nil {
		t.Fatal("Ensure() error = nil, want error")
	}
}

func layoutFor(root string) valueobject.Layout {
	dataDir := filepath.Join(root, "data")
	return valueobject.Layout{
		Root:         root,
		DataDir:      dataDir,
		LogsDir:      filepath.Join(root, "logs"),
		WebView2Dir:  filepath.Join(root, "webview2"),
		LogFile:      filepath.Join(root, "logs", "app.log"),
		RootSource:   "exe",
		Portable:     true,
		AgentDir:     filepath.Join(dataDir, "agent"),
		WorkspaceDir: filepath.Join(dataDir, "workspace", "chain_default"),
		SessionsFile: filepath.Join(dataDir, "sessions.json"),
		AuditFile:    filepath.Join(root, "logs", "audit.log"),
		ConfigFile:   filepath.Join(dataDir, "config.json"),
		ProjectsDir:  filepath.Join(dataDir, "projects"),
		RuntimeDir:   filepath.Join(root, "runtime"),
		BackupDir:    filepath.Join(dataDir, "backup", "m1"),
	}
}
