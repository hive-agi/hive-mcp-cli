package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CloneStep clones required repositories
type CloneStep struct {
	HiveMCPDir string // Target directory for hive-mcp
}

// DefaultHiveMCPDir returns the default installation directory
func DefaultHiveMCPDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "hive-mcp")
}

func (s *CloneStep) Name() string {
	return "Clone hive-mcp repository"
}

func (s *CloneStep) targetDir() string {
	if s.HiveMCPDir != "" {
		return expandPath(s.HiveMCPDir)
	}
	return DefaultHiveMCPDir()
}

func (s *CloneStep) Check() (bool, error) {
	dir := s.targetDir()

	// Check if directory exists and has .git
	gitDir := filepath.Join(dir, ".git")
	if _, err := os.Stat(gitDir); err == nil {
		return true, nil
	}
	return false, nil
}

func (s *CloneStep) Run() error {
	dir := s.targetDir()

	// Ensure parent directory exists
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Clone with submodules (recursive)
	cmd := exec.Command("git", "clone", "--recursive",
		"https://github.com/BuddhiLW/hive-mcp.git",
		dir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git clone failed: %w", err)
	}

	return nil
}

func (s *CloneStep) Rollback() error {
	dir := s.targetDir()

	// Only remove if it exists
	if _, err := os.Stat(dir); err == nil {
		return os.RemoveAll(dir)
	}
	return nil
}

// CloneDepsStep handles downloading Clojure dependencies
type CloneDepsStep struct {
	HiveMCPDir string
}

func (s *CloneDepsStep) Name() string {
	return "Download Clojure dependencies"
}

func (s *CloneDepsStep) targetDir() string {
	if s.HiveMCPDir != "" {
		return expandPath(s.HiveMCPDir)
	}
	return DefaultHiveMCPDir()
}

func (s *CloneDepsStep) Check() (bool, error) {
	// Dependencies should be re-checked each time
	// Could check for .cpcache but that's fragile
	return false, nil
}

func (s *CloneDepsStep) Run() error {
	dir := s.targetDir()

	// Run clojure -P to download dependencies
	cmd := exec.Command("clojure", "-P")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clojure -P failed: %w", err)
	}

	return nil
}

func (s *CloneDepsStep) Rollback() error {
	// Can't really rollback downloaded deps
	return nil
}
