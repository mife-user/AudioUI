package agenthost

import (
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
	return run(native, args)
}
