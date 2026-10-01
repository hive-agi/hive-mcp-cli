package setup

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// The hive-mcp JVM (bin/hive-mcp-foss) is the backend bb-mcp talks to over
// nREPL. MCP clients launch bb-mcp, not the JVM, so something has to start
// the JVM: `hive start`.

// DefaultNreplPort is the port the launcher binds unless HIVE_NREPL_PORT says
// otherwise, and the one bb-mcp falls back to.
const DefaultNreplPort = "7910"

// BackendNreplAddr is where the backend listens.
func BackendNreplAddr(getenv func(string) string) string {
	port := getenv("HIVE_NREPL_PORT")
	if port == "" {
		port = DefaultNreplPort
	}
	return "127.0.0.1:" + port
}

// BackendLogPath is where a detached backend writes its output.
func BackendLogPath(home string) string {
	return filepath.Join(home, ".local", "state", "hive-mcp", "backend.log")
}

// DetachedBackendArgv keeps the launcher's stdin open forever: it is a stdio
// MCP server too, and an EOF on stdin would end it.
func DetachedBackendArgv(launcher string) []string {
	return []string{"sh", "-c", `tail -f /dev/null | exec "$0"`, launcher}
}

// NreplListening reports whether something accepts connections at addr.
func NreplListening(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// StartBackend starts the hive-mcp JVM from hiveMCPDir. Foreground runs it
// attached to this terminal; otherwise it is detached into its own session
// with output in BackendLogPath. A backend already listening is left alone.
func StartBackend(hiveMCPDir string, foreground bool) error {
	addr := BackendNreplAddr(os.Getenv)
	if NreplListening(addr) {
		fmt.Printf("hive-mcp is already running (nREPL on %s)\n", addr)
		return nil
	}
	launcher := LauncherPath(hiveMCPDir)
	if info, err := os.Stat(launcher); err != nil || info.Mode()&0o111 == 0 {
		return fmt.Errorf("launcher not found or not executable at %s (run hive setup)", launcher)
	}

	if foreground {
		cmd := exec.Command(launcher)
		cmd.Dir = hiveMCPDir
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stderr, os.Stderr
		return cmd.Run()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	logPath := BackendLogPath(home)
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer logFile.Close()

	argv := DetachedBackendArgv(launcher)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = hiveMCPDir
	cmd.Stdout, cmd.Stderr = logFile, logFile
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", launcher, err)
	}
	fmt.Printf("hive-mcp starting in the background (pid %d); nREPL on %s in about a minute\n", cmd.Process.Pid, addr)
	fmt.Printf("log: %s\n", logPath)
	return cmd.Process.Release()
}
