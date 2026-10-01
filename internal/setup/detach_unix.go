//go:build unix

package setup

import (
	"os/exec"
	"syscall"
)

// detach puts the child in its own session, so closing this terminal does
// not take the backend down with it.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
