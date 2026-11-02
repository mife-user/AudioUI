//go:build !windows

package claudecli

import "os/exec"

func hideConsole(_ *exec.Cmd) {}
