package claudecli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrAgentUnavailable = errors.New("AGENT_UNAVAILABLE")
	versionPattern      = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
	absoluteExePattern  = regexp.MustCompile(`([A-Za-z]:\\[^"\r\n]*?claude\.exe)`)
)

const (
	minimumCLIVersion    = "2.1.224"
	nativeResolveTimeout = 10 * time.Second
)

type NativeCLI struct {
	Path    string
	Version string
}

func ResolveNative(ctx context.Context, explicit string) (NativeCLI, error) {
	ctx, cancel := context.WithTimeout(ctx, nativeResolveTimeout)
	defer cancel()
	candidates := candidatePaths(explicit)
	seen := map[string]bool{}
	var lastErr error
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if seen[strings.ToLower(candidate)] {
			continue
		}
		seen[strings.ToLower(candidate)] = true
		lower := strings.ToLower(candidate)
		if strings.HasSuffix(lower, ".cmd") || strings.HasSuffix(lower, ".bat") {
			lastErr = fmt.Errorf("%w: refusing batch shim %s; use a native claude.exe", ErrAgentUnavailable, candidate)
			continue
		}
		if !strings.HasSuffix(lower, ".exe") {
			continue
		}
		if info, err := os.Stat(candidate); err != nil || info.IsDir() {
			continue
		}
		version, err := probeVersion(ctx, candidate)
		if err != nil {
			lastErr = err
			continue
		}
		if !versionAtLeast(version, minimumCLIVersion) {
			return NativeCLI{}, fmt.Errorf("%w: Claude CLI %s is older than %s", ErrAgentUnavailable, version, minimumCLIVersion)
		}
		return NativeCLI{Path: candidate, Version: version}, nil
	}
	if lastErr != nil {
		return NativeCLI{}, lastErr
	}
	return NativeCLI{}, fmt.Errorf("%w: native Claude CLI not found; install Claude Code natively: irm https://claude.ai/install.ps1 | iex", ErrAgentUnavailable)
}

func candidatePaths(explicit string) []string {
	var paths []string
	if strings.TrimSpace(explicit) != "" {
		paths = append(paths, explicit)
	}
	if found, err := exec.LookPath("claude.exe"); err == nil {
		paths = append(paths, found)
	}
	if found, err := exec.LookPath("claude"); err == nil {
		if strings.HasSuffix(strings.ToLower(found), ".exe") {
			paths = append(paths, found)
		} else {
			paths = append(paths, shimCandidates(found)...)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".local", "bin", "claude.exe"))
	}
	return paths
}

func shimCandidates(shim string) []string {
	dir := filepath.Dir(shim)
	candidates := []string{
		filepath.Join(dir, "node_modules", "@anthropic-ai", "claude-code", "bin", "claude.exe"),
		filepath.Join(dir, "node_modules", "@anthropic-ai", "claude-code", "node_modules", "@anthropic-ai", "claude-code-win32-x64", "claude.exe"),
		filepath.Join(dir, "node_modules", "@anthropic-ai", "claude-code-win32-x64", "claude.exe"),
		filepath.Join(dir, "node_modules", "@anthropic-ai", "claude-code", "node_modules", "@anthropic-ai", "claude-code-win32-x64", "claude.exe"),
	}
	if data, err := os.ReadFile(shim); err == nil {
		text := strings.ReplaceAll(string(data), "%~dp0", dir+string(filepath.Separator))
		text = strings.ReplaceAll(text, "%dp0%", dir)
		for _, match := range absoluteExePattern.FindAllString(text, -1) {
			candidates = append(candidates, match)
		}
	}
	return candidates
}

func probeVersion(ctx context.Context, cliPath string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(probeCtx, cliPath, "--version")
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%w: cannot run %s --version: %v", ErrAgentUnavailable, cliPath, err)
	}
	match := versionPattern.FindStringSubmatch(string(out))
	if len(match) != 4 {
		return "", fmt.Errorf("%w: cannot parse Claude CLI version from %q", ErrAgentUnavailable, strings.TrimSpace(string(out)))
	}
	return strings.Join(match[1:], "."), nil
}

func versionAtLeast(got, minimum string) bool {
	gp := strings.Split(got, ".")
	mp := strings.Split(minimum, ".")
	if len(gp) < 3 || len(mp) < 3 {
		return false
	}
	for i := 0; i < 3; i++ {
		var g, m int
		_, _ = fmt.Sscanf(gp[i], "%d", &g)
		_, _ = fmt.Sscanf(mp[i], "%d", &m)
		if g > m {
			return true
		}
		if g < m {
			return false
		}
	}
	return true
}
