package setup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ShellStep exports the checkout locations in the user's shell rc files, in
// one managed block that a re-run rewrites in place.
type ShellStep struct {
	HiveMCPDir string
	// BBMCPDir overrides the bb-mcp checkout; default ResolveBBMCPDir.
	BBMCPDir string
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

func (s *ShellStep) bbMCPDir() string {
	if s.BBMCPDir != "" {
		return expandPath(s.BBMCPDir)
	}
	return ResolveBBMCPDir(DefaultBBMCPSys())
}

// envVar is one export line, in a stable order.
type envVar struct{ Key, Value string }

func (s *ShellStep) envVars() []envVar {
	return []envVar{
		{"HIVE_MCP_DIR", s.hiveMCPDir()},
		{BBMCPDirEnv, s.bbMCPDir()},
	}
}

// marker identifies hive-mcp-cli managed section
const shellMarker = "# hive-mcp-cli managed"

// renderShellBlock is the managed block for vars. Pure.
func renderShellBlock(vars []envVar) string {
	lines := []string{"", shellMarker + " - START"}
	for _, v := range vars {
		lines = append(lines, exportLine(v))
	}
	lines = append(lines, shellMarker+" - END", "")
	return strings.Join(lines, "\n")
}

func exportLine(v envVar) string {
	return fmt.Sprintf("export %s=\"%s\"", v.Key, v.Value)
}

// shellBlockCurrent: content carries a managed block exporting every var. Pure.
func shellBlockCurrent(content string, vars []envVar) bool {
	if !strings.Contains(content, shellMarker+" - START") {
		return false
	}
	for _, v := range vars {
		if !strings.Contains(content, exportLine(v)) {
			return false
		}
	}
	return true
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

// Check passes when some rc file already exports every variable. A block from
// an older CLI (HIVE_MCP_DIR only) is not current, so the step rewrites it.
func (s *ShellStep) Check() (bool, error) {
	vars := s.envVars()
	for _, f := range shellConfigFiles() {
		content, err := os.ReadFile(f)
		if err == nil && shellBlockCurrent(string(content), vars) {
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
	block := renderShellBlock(s.envVars())
	for _, f := range files {
		if err := replaceMarkedSection(f, block); err != nil {
			return fmt.Errorf("failed to write %s: %w", f, err)
		}
	}
	return nil
}

func (s *ShellStep) Rollback() error {
	for _, f := range shellConfigFiles() {
		if err := removeMarkedSection(f); err != nil {
			return fmt.Errorf("failed to rollback %s: %w", f, err)
		}
	}
	return nil
}

// replaceMarkedSection drops any managed block from path and appends block.
func replaceMarkedSection(path, block string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	kept := stripMarkedSection(string(raw))
	if kept != "" {
		kept += "\n"
	}
	return os.WriteFile(path, []byte(kept+block), 0644)
}

// removeMarkedSection removes the hive-mcp-cli managed section from a file
func removeMarkedSection(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(stripMarkedSection(string(raw))+"\n"), 0644)
}

// stripMarkedSection returns content without the managed block and without
// trailing blank lines. Pure.
func stripMarkedSection(content string) string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	inSection := false
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.Contains(line, shellMarker+" - START"):
			inSection = true
		case strings.Contains(line, shellMarker+" - END"):
			inSection = false
		case !inSection:
			lines = append(lines, line)
		}
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}
