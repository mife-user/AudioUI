//go:build windows

package agenthost

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func run(native string, args []string) int {
	cmd := exec.Command(native, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = withoutEnv(os.Environ(), EnvMode)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}

	job, jobErr := windows.CreateJobObject(nil, nil)
	if jobErr == nil {
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		_, _ = windows.SetInformationJobObject(
			job,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		)
		defer windows.CloseHandle(job)
	}

	if err := cmd.Start(); err != nil {
		_, _ = os.Stderr.WriteString("agent host: " + err.Error() + "\n")
		return 1
	}

	if jobErr == nil {
		processHandle, err := windows.OpenProcess(
			windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
			false,
			uint32(cmd.Process.Pid),
		)
		if err == nil {
			_ = windows.AssignProcessToJobObject(job, processHandle)
			_ = windows.CloseHandle(processHandle)
		}
	}

	if err := cmd.Wait(); err != nil {
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
