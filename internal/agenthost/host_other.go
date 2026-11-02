//go:build !windows

package agenthost

import (
	"os"
	"os/exec"
	"strings"
)

func run(native string, args []string) int {
	cmd := exec.Command(native, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = withoutEnv(os.Environ(), EnvMode)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		_, _ = os.Stderr.WriteString("agent host: " + err.Error() + "\n")
		return 1
	}
	return 0
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env))
	for _, item := range env {
		if strings.HasPrefix(item, prefix) {
			continue
		}
		result = append(result, item)
	}
	return result
}
