package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellBlockExportsBothCheckoutsInOrder(t *testing.T) {
	block := renderShellBlock([]envVar{{"HIVE_MCP_DIR", "/h/hive-mcp"}, {"BB_MCP_DIR", "/h/bb-mcp"}})
	hive := strings.Index(block, `export HIVE_MCP_DIR="/h/hive-mcp"`)
	bb := strings.Index(block, `export BB_MCP_DIR="/h/bb-mcp"`)
	if hive < 0 || bb < 0 || bb < hive {
		t.Errorf("block:\n%s", block)
	}
}

func TestAnOldBlockWithoutBBMCPDirIsNotCurrent(t *testing.T) {
	vars := []envVar{{"HIVE_MCP_DIR", "/h/hive-mcp"}, {"BB_MCP_DIR", "/h/bb-mcp"}}
	old := renderShellBlock(vars[:1])
	if shellBlockCurrent(old, vars) {
		t.Error("a block from an older CLI must be rewritten")
	}
	if !shellBlockCurrent(renderShellBlock(vars), vars) {
		t.Error("a fresh block must be current")
	}
}

func TestShellStepRewritesItsBlockInPlace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	rc := filepath.Join(home, ".bashrc")
	user := "# mine\nalias ll='ls -l'\n"
	old := user + renderShellBlock([]envVar{{"HIVE_MCP_DIR", "/old"}})
	os.WriteFile(rc, []byte(old), 0o644)

	step := &ShellStep{HiveMCPDir: "/h/hive-mcp", BBMCPDir: "/h/bb-mcp"}
	if done, _ := step.Check(); done {
		t.Fatal("old block reported current")
	}
	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	if err := step.Run(); err != nil { // idempotent
		t.Fatal(err)
	}
	b, _ := os.ReadFile(rc)
	got := string(b)
	if !strings.HasPrefix(got, user) {
		t.Errorf("user lines lost:\n%s", got)
	}
	if strings.Count(got, shellMarker+" - START") != 1 || strings.Contains(got, `"/old"`) {
		t.Errorf("block not replaced in place:\n%s", got)
	}
	if done, _ := step.Check(); !done {
		t.Errorf("after Run the block must be current:\n%s", got)
	}
}
