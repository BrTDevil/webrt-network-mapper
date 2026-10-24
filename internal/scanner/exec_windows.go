//go:build windows

package scanner

import (
	"os/exec"
	"syscall"
)

// hideWindow ascunde fereastra de consolă a proceselor copil.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
