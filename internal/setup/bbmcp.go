package setup

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// bb-mcp is the MCP server every client launches. It is a thin babashka
// process that talks to the hive-mcp JVM over nREPL (7910). Wherever bb-mcp is
// checked out is the reference, published at ONE well-known anchor; client
// registration lives ONLY in `<checkout>/bb-mcp setup`. This file decides
// which checkout that is (cloning one when there is none) and hands over.
//
// Strata, CPPB style:
//   collect  ObserveBBMCP     reads the filesystem through BBMCPSys
//   promote  PlanBBMCP        pure: observed state -> plan
//            LegacyHiveEntries pure: ~/.claude.json bytes -> stale registrations
//   boundary BBMCPStep.Run    executes the plan through BBMCPSys

const (
	// BBMCPRepoURL is cloned when no checkout is published at the anchor.
	BBMCPRepoURL = "https://github.com/hive-agi/bb-mcp"
	// BBMCPAnchorRel is the anchor relative to $HOME. A symlink to the real
	// checkout, created and owned by `bb-mcp setup`.
	BBMCPAnchorRel = ".local/share/hive-mcp/bb-mcp"
	// BBMCPDirEnv overrides where a fresh clone goes (default ~/bb-mcp).
	BBMCPDirEnv = "BB_MCP_DIR"
	// legacyLauncher is what `hive setup` used to register directly.
	legacyLauncher = "hive-mcp-foss"
)

// BBMCPAnchor is the anchor path for a given home directory.
func BBMCPAnchor(home string) string {
	return filepath.Join(home, BBMCPAnchorRel)
}

// BBMCPSetupArgv is the command that registers hive with every MCP client.
func BBMCPSetupArgv(checkout string, args ...string) []string {
	return append([]string{filepath.Join(checkout, "bb-mcp"), "setup"}, args...)
}

// --- effects port ----------------------------------------------------------

// BBMCPSys is every effect the bb-mcp step performs. Tests replace the fields
// they care about; DefaultBBMCPSys is the real machine.
type BBMCPSys struct {
	Home         func() (string, error)
	Getenv       func(string) string
	EvalSymlinks func(string) (string, error)
	Stat         func(string) (os.FileInfo, error)
	ReadFile     func(string) ([]byte, error)
	MkdirAll     func(string, os.FileMode) error
	// Run executes argv with the user's terminal attached.
	Run func(argv ...string) error
	// Say reports one line of progress.
	Say func(string)
}

// DefaultBBMCPSys wires BBMCPSys to the OS.
func DefaultBBMCPSys() BBMCPSys {
	return BBMCPSys{
		Home:         os.UserHomeDir,
		Getenv:       os.Getenv,
		EvalSymlinks: filepath.EvalSymlinks,
		Stat:         os.Stat,
		ReadFile:     os.ReadFile,
		MkdirAll:     os.MkdirAll,
		Run:          runAttached,
		Say:          func(s string) { fmt.Println("  " + s) },
	}
}

func runAttached(argv ...string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// --- collect ---------------------------------------------------------------

// BBMCPObserved is the machine state the plan is made from.
type BBMCPObserved struct {
	Anchor string
	// AnchorCheckout is the real directory the anchor resolves to, when that
	// directory is a bb-mcp checkout; empty otherwise.
	AnchorCheckout string
	// Target is ${BB_MCP_DIR:-$HOME/bb-mcp}, where a fresh clone goes.
	Target           string
	TargetExists     bool
	TargetIsCheckout bool
	// Legacy lists registrations of the old direct launcher.
	Legacy []LegacyEntry
}

// isBBMCPCheckout: a directory carrying both the shim and the launcher.
func (sys BBMCPSys) isBBMCPCheckout(dir string) bool {
	for _, f := range []string{"bb-mcp", "start-bb-mcp.sh"} {
		info, err := sys.Stat(filepath.Join(dir, f))
		if err != nil || info.IsDir() {
			return false
		}
	}
	return true
}

// ResolveBBMCPDir answers which checkout is (or will be) the reference: the
// anchor's target when it resolves to a checkout, else ${BB_MCP_DIR:-~/bb-mcp}.
func ResolveBBMCPDir(sys BBMCPSys) string {
	o := ObserveBBMCP(sys)
	if o.AnchorCheckout != "" {
		return o.AnchorCheckout
	}
	return o.Target
}

// ObserveBBMCP collects the state PlanBBMCP needs.
func ObserveBBMCP(sys BBMCPSys) BBMCPObserved {
	home, _ := sys.Home()
	o := BBMCPObserved{Anchor: BBMCPAnchor(home)}

	if real, err := sys.EvalSymlinks(o.Anchor); err == nil && sys.isBBMCPCheckout(real) {
		o.AnchorCheckout = real
	}

	o.Target = sys.Getenv(BBMCPDirEnv)
	if o.Target == "" {
		o.Target = filepath.Join(home, "bb-mcp")
	}
	o.Target = expandPath(o.Target)
	if _, err := sys.Stat(o.Target); err == nil {
		o.TargetExists = true
		o.TargetIsCheckout = sys.isBBMCPCheckout(o.Target)
	}

	if raw, err := sys.ReadFile(filepath.Join(home, ".claude.json")); err == nil {
		o.Legacy = LegacyHiveEntries(raw)
	}
	return o
}

// --- promote (pure) --------------------------------------------------------

// BBMCPPlan is what the step will do.
type BBMCPPlan struct {
	Checkout string
	Clone    bool
	Setup    []string
	Notes    []string
}

// PlanBBMCP decides, from observed state alone, which checkout becomes the
// reference and what runs. Precedence: the anchor (wherever bb-mcp already
// lives wins), then an existing checkout at the target, then a fresh clone.
// A target that exists but is not a checkout is refused, never overwritten.
func PlanBBMCP(o BBMCPObserved) (BBMCPPlan, error) {
	var p BBMCPPlan
	switch {
	case o.AnchorCheckout != "":
		p.Checkout = o.AnchorCheckout
		p.Notes = append(p.Notes, "reusing the bb-mcp published at "+o.Anchor+" -> "+o.AnchorCheckout)
	case o.TargetIsCheckout:
		p.Checkout = o.Target
		p.Notes = append(p.Notes, "reusing the bb-mcp checkout at "+o.Target)
	case o.TargetExists:
		return p, fmt.Errorf("%s exists but is not a bb-mcp checkout; move it aside or set %s to another path", o.Target, BBMCPDirEnv)
	default:
		p.Checkout = o.Target
		p.Clone = true
		p.Notes = append(p.Notes, "cloning "+BBMCPRepoURL+" to "+o.Target)
	}
	p.Setup = BBMCPSetupArgv(p.Checkout, "--client", "all")
	p.Notes = append(p.Notes, legacyNotes(o.Legacy)...)
	return p, nil
}

// LegacyEntry is a "hive" registration that still launches bin/hive-mcp-foss
// directly. Scope "user" is replaced by `bb-mcp setup`; a "local" one shadows
// the user entry in its project and must be removed from that project.
type LegacyEntry struct {
	Scope   string // "user" or "local"
	Project string // for scope local
	Command string
}

// LegacyHiveEntries finds every hive entry in ~/.claude.json that points at
// the old launcher. Unparseable input yields none.
func LegacyHiveEntries(claudeJSON []byte) []LegacyEntry {
	type servers map[string]struct {
		Command string `json:"command"`
	}
	var cfg struct {
		MCPServers servers `json:"mcpServers"`
		Projects   map[string]struct {
			MCPServers servers `json:"mcpServers"`
		} `json:"projects"`
	}
	if json.Unmarshal(claudeJSON, &cfg) != nil {
		return nil
	}
	isLegacy := func(cmd string) bool { return strings.HasSuffix(cmd, legacyLauncher) }

	var out []LegacyEntry
	if c := cfg.MCPServers[MCPServerName].Command; isLegacy(c) {
		out = append(out, LegacyEntry{Scope: "user", Command: c})
	}
	projects := make([]string, 0, len(cfg.Projects))
	for p := range cfg.Projects {
		projects = append(projects, p)
	}
	sort.Strings(projects)
	for _, p := range projects {
		if c := cfg.Projects[p].MCPServers[MCPServerName].Command; isLegacy(c) {
			out = append(out, LegacyEntry{Scope: "local", Project: p, Command: c})
		}
	}
	return out
}

// LegacyRemedy is the one line that clears a legacy entry.
func LegacyRemedy(e LegacyEntry) string {
	if e.Scope == "user" {
		return "replaced by bb-mcp setup (user scope)"
	}
	return fmt.Sprintf("cd %q && claude mcp remove --scope local %s", e.Project, MCPServerName)
}

func legacyNotes(entries []LegacyEntry) []string {
	var notes []string
	for _, e := range entries {
		where := "user scope"
		if e.Scope == "local" {
			where = "project " + e.Project
		}
		notes = append(notes, fmt.Sprintf("migrating: old hive entry (%s, %s): %s", where, e.Command, LegacyRemedy(e)))
	}
	return notes
}

// --- boundary --------------------------------------------------------------

// BBMCPStep ensures a bb-mcp checkout and lets `bb-mcp setup` publish the
// anchor and register hive with every MCP client.
type BBMCPStep struct {
	// Sys defaults to DefaultBBMCPSys when nil.
	Sys *BBMCPSys
}

func (s *BBMCPStep) sys() BBMCPSys {
	if s.Sys != nil {
		return *s.Sys
	}
	return DefaultBBMCPSys()
}

func (s *BBMCPStep) Name() string {
	return "Register hive with your MCP clients (bb-mcp setup)"
}

// Check asks bb-mcp itself: `--check` exits zero only when the anchor and
// every detected client's entry are right. A legacy entry anywhere means the
// migration still has something to say, so the step runs.
func (s *BBMCPStep) Check() (bool, error) {
	sys := s.sys()
	o := ObserveBBMCP(sys)
	if o.AnchorCheckout == "" || len(o.Legacy) > 0 {
		return false, nil
	}
	return sys.Run(BBMCPSetupArgv(o.AnchorCheckout, "--check")...) == nil, nil
}

func (s *BBMCPStep) Run() error {
	sys := s.sys()
	plan, err := PlanBBMCP(ObserveBBMCP(sys))
	if err != nil {
		return err
	}
	for _, n := range plan.Notes {
		sys.Say(n)
	}
	if plan.Clone {
		if err := sys.MkdirAll(filepath.Dir(plan.Checkout), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(plan.Checkout), err)
		}
		if err := sys.Run("git", "clone", BBMCPRepoURL, plan.Checkout); err != nil {
			return fmt.Errorf("git clone %s failed: %w", BBMCPRepoURL, err)
		}
	}
	if err := sys.Run(plan.Setup...); err != nil {
		return fmt.Errorf("%s failed: %w", strings.Join(plan.Setup, " "), err)
	}
	return nil
}

// Rollback leaves the checkout and the registrations alone: both belong to
// bb-mcp, and a clone may be the user's working copy by now.
func (s *BBMCPStep) Rollback() error { return nil }
