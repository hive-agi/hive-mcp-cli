package setup

import (
	"fmt"
	"os"
	"os/exec"
)

// DoomSyncStep runs doom sync for Emacs packages
type DoomSyncStep struct{}

func (s *DoomSyncStep) Name() string {
	return "Sync Doom Emacs packages"
}

func (s *DoomSyncStep) Check() (bool, error) {
	// Always run doom sync to ensure packages are current
	return false, nil
}

func (s *DoomSyncStep) Run() error {
	// Find doom command
	home, _ := os.UserHomeDir()
	doomPaths := []string{
		home + "/.emacs.d/bin/doom",
		home + "/.config/emacs/bin/doom",
	}

	var doomCmd string
	for _, p := range doomPaths {
		if _, err := os.Stat(p); err == nil {
			doomCmd = p
			break
		}
	}

	if doomCmd == "" {
		return fmt.Errorf("doom command not found - is Doom Emacs installed?")
	}

	cmd := exec.Command(doomCmd, "sync")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("doom sync failed: %w", err)
	}

	return nil
}

func (s *DoomSyncStep) Rollback() error {
	// Can't rollback doom sync
	return nil
}
