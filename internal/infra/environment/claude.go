package environment

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"audio-ui/internal/domain/valueobject"
)

const cliTimeout = 2 * time.Second

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

// ClaudeProbe shells out to the installed Claude Code CLI. Missing CLI is a
// warning, never a startup failure.
type ClaudeProbe struct {
	LookPath func(string) (string, error)
	Run      func(context.Context, string) ([]byte, error)
}

func NewClaudeProbe() *ClaudeProbe {
	return &ClaudeProbe{
		LookPath: exec.LookPath,
		Run:      runClaudeVersion,
	}
}

func (p *ClaudeProbe) Detect(ctx context.Context) valueobject.CLIStatus {
	path, err := p.LookPath("claude")
	if err != nil || path == "" {
		return valueobject.CLIStatus{Error: "未找到 claude 命令"}
	}

	probeCtx, cancel := context.WithTimeout(ctx, cliTimeout)
	defer cancel()

	output, runErr := p.Run(probeCtx, path)
	if runErr != nil {
		return valueobject.CLIStatus{
			Installed: true,
			Path:      path,
			Version:   "已安装",
			Error:     "版本检测失败",
		}
	}
	version := firstNonEmptyLine(string(output))
	if version == "" {
		version = "已安装"
	}
	return valueobject.CLIStatus{
		Installed: true,
		Path:      path,
		Version:   version,
	}
}

func runClaudeVersion(ctx context.Context, path string) ([]byte, error) {
	cmd := claudeCommand(ctx, path)
	configureCommand(cmd)
	return cmd.CombinedOutput()
}

func claudeCommand(ctx context.Context, path string) *exec.Cmd {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
		return exec.CommandContext(ctx, "cmd.exe", "/d", "/s", "/c", "call", path, "--version")
	}
	return exec.CommandContext(ctx, path, "--version")
}

func firstNonEmptyLine(output string) string {
	clean := ansiPattern.ReplaceAllString(output, "")
	for _, line := range strings.Split(strings.ReplaceAll(clean, "\r\n", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
