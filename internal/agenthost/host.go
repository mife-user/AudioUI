package agenthost

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

const (
	EnvMode   = "AUDIOUI_AGENT_HOST"
	EnvNative = "AUDIOUI_NATIVE_CLAUDE"
)

// IsHost reports whether the current process was launched as an agent host.
func IsHost() bool {
	return os.Getenv(EnvMode) == "1"
}

// Run forwards the process stdio to the native Claude executable.
func Run(args []string) int {
	native := strings.TrimSpace(os.Getenv(EnvNative))
	if native == "" {
		_, _ = os.Stderr.WriteString("agent host: native Claude path is not configured\n")
		return 2
	}
	if path, ok := mcpConfigArgument(args); ok {
		if err := validateMCPConfigPath(path); err != nil {
			_, _ = os.Stderr.WriteString("agent host: " + err.Error() + "\n")
			return 3
		}
	}
	return run(native, args)
}

func mcpConfigArgument(args []string) (string, bool) {
	for i, arg := range args {
		if arg == "--mcp-config" {
			if i+1 < len(args) {
				return args[i+1], true
			}
			return "", true
		}
		if value, ok := strings.CutPrefix(arg, "--mcp-config="); ok {
			return value, true
		}
	}
	return "", false
}

func validateMCPConfigPath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("MCP_CONFIG_NOT_FOUND: MCP config path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("MCP_CONFIG_NOT_FOUND: %s", path)
		}
		return fmt.Errorf("MCP_CONFIG_UNREADABLE: %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("MCP_CONFIG_INVALID: not a regular file: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("MCP_CONFIG_UNREADABLE: %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("MCP_CONFIG_UNREADABLE: %s: %w", path, err)
	}
	return nil
}
