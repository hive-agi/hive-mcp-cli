package setup

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

// EmacsDaemonStep starts the Emacs daemon
type EmacsDaemonStep struct{}

func (s *EmacsDaemonStep) Name() string {
	return "Start Emacs daemon"
}

func (s *EmacsDaemonStep) Check() (bool, error) {
	// Check if Emacs daemon is running
	cmd := exec.Command("emacsclient", "-e", "(emacs-pid)")
	if err := cmd.Run(); err == nil {
		return true, nil
	}
	return false, nil
}

func (s *EmacsDaemonStep) Run() error {
	// Start Emacs daemon
	cmd := exec.Command("emacs", "--daemon")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start Emacs daemon: %w", err)
	}

	// Wait briefly for daemon to initialize
	time.Sleep(2 * time.Second)

	// Verify it's running
	done, _ := s.Check()
	if !done {
		return fmt.Errorf("Emacs daemon started but not responding")
	}

	return nil
}

func (s *EmacsDaemonStep) Rollback() error {
	// Kill emacs daemon
	exec.Command("emacsclient", "-e", "(kill-emacs)").Run()
	return nil
}
