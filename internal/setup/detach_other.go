//go:build !unix

package setup

import "os/exec"

func detach(cmd *exec.Cmd) {}
