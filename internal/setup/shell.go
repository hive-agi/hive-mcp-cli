package setup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ShellStep configures shell environment variables
type ShellStep struct {
	HiveMCPDir string
}

func (s *ShellStep) Name() string {
	return "Configure shell environment"
}

func (s *ShellStep) hiveMCPDir() string {
	if s.HiveMCPDir != "" {
		return expandPath(s.HiveMCPDir)
	}
	return DefaultHiveMCPDir()
}

// envVars returns the environment variables to set
func (s *ShellStep) envVars() map[string]string {
	hiveMCP := s.hiveMCPDir()
	return map[string]string{
		"HIVE_MCP_DIR": hiveMCP,
		"BB_MCP_DIR":   hiveMCP,
	}
}

// shellConfigFiles returns paths to shell config files that exist
func shellConfigFiles() []string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		filepath.Join(home, ".bashrc"),
		filepath.Join(home, ".zshrc"),
	}

	var existing []string
	for _, f := range candidates {
		if _, err := os.Stat(f); err == nil {
			existing = append(existing, f)
		}
	}
	return existing
}

// marker identifies hive-mcp-cli managed section
const shellMarker = "# hive-mcp-cli managed"

func (s *ShellStep) Check() (bool, error) {
	files := shellConfigFiles()
	if len(files) == 0 {
		return false, nil
	}

	// Check if any shell config already has our marker
	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if strings.Contains(string(content), shellMarker) {
			return true, nil
		}
	}
	return false, nil
}

func (s *ShellStep) Run() error {
	files := shellConfigFiles()
	if len(files) == 0 {
		return fmt.Errorf("no shell config found (.bashrc or .zshrc)")
	}

	// Build the config block
	vars := s.envVars()
	var lines []string
	lines = append(lines, "", shellMarker+" - START")
	for key, value := range vars {
		lines = append(lines, fmt.Sprintf("export %s=\"%s\"", key, value))
	}
	lines = append(lines, shellMarker+" - END", "")
	block := strings.Join(lines, "\n")

	// Append to each shell config
	for _, f := range files {
		file, err := os.OpenFile(f, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", f, err)
		}
		defer file.Close()

		if _, err := file.WriteString(block); err != nil {
			return fmt.Errorf("failed to write to %s: %w", f, err)
		}
	}

	return nil
}

func (s *ShellStep) Rollback() error {
	files := shellConfigFiles()

	for _, f := range files {
		if err := removeMarkedSection(f); err != nil {
			return fmt.Errorf("failed to rollback %s: %w", f, err)
		}
	}
	return nil
}

// removeMarkedSection removes the hive-mcp-cli managed section from a file
func removeMarkedSection(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	inSection := false

	for scanner.Scan() {
		line := scanner.Text()

		if strings.Contains(line, shellMarker+" - START") {
			inSection = true
			continue
		}
		if strings.Contains(line, shellMarker+" - END") {
			inSection = false
			continue
		}
		if !inSection {
			lines = append(lines, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Remove trailing empty lines that we added
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644)
}
