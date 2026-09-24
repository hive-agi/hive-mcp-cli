package auth

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
)

// CanOpenBrowser reports whether a browser on THIS machine is plausible. Over
// ssh, or on Linux with no display, the loopback flow would open nothing (or a
// browser on a machine the user is not looking at), so the device flow is used.
func CanOpenBrowser() bool {
	if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
		return false
	}
	if os.Getenv("BROWSER") != "" {
		return true
	}
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	default:
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			return false
		}
		_, err := exec.LookPath("xdg-open")
		return err == nil
	}
}

// OpenBrowser opens url without waiting for the browser to exit. $BROWSER wins,
// as it does for gh and git.
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch {
	case os.Getenv("BROWSER") != "":
		cmd = exec.Command(os.Getenv("BROWSER"), url)
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", url)
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return errors.New("no xdg-open")
		}
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
