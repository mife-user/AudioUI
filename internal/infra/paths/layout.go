package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"audio-ui/internal/domain/valueobject"
)

const homeEnv = "AUDIOUI_HOME"

// Resolver resolves the portable application root.
// Tests inject the executable and environment lookups.
type Resolver struct {
	executable func() (string, error)
	getenv     func(string) string
}

func NewResolver() *Resolver {
	return &Resolver{
		executable: os.Executable,
		getenv:     os.Getenv,
	}
}

func NewResolverWith(executable func() (string, error), getenv func(string) string) *Resolver {
	return &Resolver{executable: executable, getenv: getenv}
}

func (r *Resolver) Resolve() (valueobject.Layout, error) {
	var root string
	source := "exe"
	portable := true

	if override := strings.TrimSpace(r.getenv(homeEnv)); override != "" {
		absolute, err := filepath.Abs(override)
		if err != nil {
			return valueobject.Layout{}, fmt.Errorf("无法解析 %s: %w", homeEnv, err)
		}
		root = absolute
		source = "env"
		portable = false
	} else {
		executable, err := r.executable()
		if err != nil {
			return valueobject.Layout{}, fmt.Errorf("无法定位可执行文件: %w", err)
		}
		root = filepath.Dir(executable)
	}

	info, err := os.Stat(root)
	if err != nil {
		return valueobject.Layout{}, fmt.Errorf("无法访问程序目录 %s: %w", root, err)
	}
	if !info.IsDir() {
		return valueobject.Layout{}, fmt.Errorf("程序目录不是文件夹: %s", root)
	}

	logsDir := filepath.Join(root, "logs")
	dataDir := filepath.Join(root, "data")
	webview2Dir := filepath.Join(root, "webview2")
	agentDir := filepath.Join(dataDir, "agent")
	workspaceDir := filepath.Join(dataDir, "workspace", "chain_default")
	projectsDir := filepath.Join(dataDir, "projects")

	return valueobject.Layout{
		Root:         root,
		DataDir:      dataDir,
		LogsDir:      logsDir,
		WebView2Dir:  webview2Dir,
		LogFile:      filepath.Join(logsDir, "app.log"),
		RootSource:   source,
		Portable:     portable,
		AgentDir:     agentDir,
		WorkspaceDir: workspaceDir,
		SessionsFile: filepath.Join(dataDir, "sessions.json"),
		AuditFile:    filepath.Join(logsDir, "audit.log"),
		ConfigFile:   filepath.Join(dataDir, "config.json"),
		ProjectsDir:  projectsDir,
		RuntimeDir:   filepath.Join(root, "runtime"),
		BackupDir:    filepath.Join(dataDir, "backup", "m1"),
	}, nil
}

// Ensure creates the portable directories and verifies that each is writable.
// It never falls back to AppData.
func Ensure(layout valueobject.Layout) error {
	dirs := []string{layout.DataDir, layout.LogsDir, layout.WebView2Dir}
	for _, dir := range []string{layout.AgentDir, layout.WorkspaceDir, layout.ProjectsDir, layout.BackupDir} {
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("无法创建目录 %s: %w", dir, err)
		}
		if err := probeWritable(dir); err != nil {
			return fmt.Errorf("目录不可写 %s: %w", dir, err)
		}
	}
	return nil
}

func probeWritable(dir string) error {
	file, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return err
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}
