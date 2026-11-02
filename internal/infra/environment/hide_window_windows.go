//go:build windows

package environment

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP,
	}
	cmd.WaitDelay = 500 * time.Millisecond
	cmd.Cancel = func() error {
		return killProcessTree(cmd.Process)
	}
}

func killProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	taskkill := "taskkill.exe"
	if root := os.Getenv("SystemRoot"); root != "" {
		taskkill = filepath.Join(root, "System32", "taskkill.exe")
	}
	kill := exec.Command(taskkill, "/T", "/F", "/PID", strconv.Itoa(process.Pid))
	kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := kill.Run(); err != nil {
		return process.Kill()
	}
	return nil
}
