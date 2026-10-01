package setup

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// `hive update` brings a checkout up to date with its tracked branch, and
// nothing else: it never stashes, never rebases, never switches branch. A
// checkout that is not in a state a fast-forward can resolve is refused with
// the commands that would get it there.
//
// Strata, CPPB style:
//   collect  ObserveRepo        reads git state through UpdateSys
//   promote  PlanRepoUpdate     pure: observed state -> plan or refusal
//            PlanUpdate         pure: which repos, in which order, and the check
//   boundary RunUpdate          executes the plan through UpdateSys

// HiveMCPDirEnv names the hive-mcp checkout `hive setup` clones.
const HiveMCPDirEnv = "HIVE_MCP_DIR"

// --- effects port ----------------------------------------------------------

// UpdateSys is every effect `hive update` performs. Tests replace the fields
// they care about; DefaultUpdateSys is the real machine.
type UpdateSys struct {
	Home         func() (string, error)
	Getenv       func(string) string
	EvalSymlinks func(string) (string, error)
	Stat         func(string) (os.FileInfo, error)
	// Output runs argv and returns its trimmed stdout (queries only).
	Output func(argv ...string) (string, error)
	// Run executes argv with the user's terminal attached.
	Run func(argv ...string) error
	// Say reports one line of progress.
	Say func(string)
}

// DefaultUpdateSys wires UpdateSys to the OS.
func DefaultUpdateSys() UpdateSys {
	return UpdateSys{
		Home:         os.UserHomeDir,
		Getenv:       os.Getenv,
		EvalSymlinks: filepath.EvalSymlinks,
		Stat:         os.Stat,
		Output:       runOutput,
		Run:          runAttached,
		Say:          func(s string) { fmt.Println("  " + s) },
	}
}

func runOutput(argv ...string) (string, error) {
	out, err := exec.Command(argv[0], argv[1:]...).Output()
	return strings.TrimSpace(string(out)), err
}

// bbmcpSys projects the read-only part of UpdateSys onto BBMCPSys so the
// anchor is resolved by the same code `hive setup` uses.
func (sys UpdateSys) bbmcpSys() BBMCPSys {
	return BBMCPSys{
		Home:         sys.Home,
		Getenv:       sys.Getenv,
		EvalSymlinks: sys.EvalSymlinks,
		Stat:         sys.Stat,
		ReadFile:     func(string) ([]byte, error) { return nil, os.ErrNotExist },
	}
}

// --- collect ---------------------------------------------------------------

// RepoState is what a fast-forward decision needs about one checkout.
type RepoState struct {
	Name string
	Dir  string
	// Present is false when Dir is not a git checkout.
	Present bool
	// Branch is the checked-out branch, "HEAD" when detached.
	Branch string
	// Upstream is the tracked ref (e.g. origin/main), empty when none.
	Upstream string
	// Dirty lists `git status --porcelain` lines for tracked paths. Untracked
	// files do not block: a fast-forward that would overwrite one fails on
	// its own, without touching the checkout.
	Dirty []string
}

func gitArgv(dir string, args ...string) []string {
	return append([]string{"git", "-C", dir}, args...)
}

// ObserveRepo collects the git state of dir.
func ObserveRepo(sys UpdateSys, name, dir string) RepoState {
	st := RepoState{Name: name, Dir: dir}
	if _, err := sys.Stat(filepath.Join(dir, ".git")); err != nil {
		return st
	}
	st.Present = true
	st.Branch, _ = sys.Output(gitArgv(dir, "rev-parse", "--abbrev-ref", "HEAD")...)
	if up, err := sys.Output(gitArgv(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")...); err == nil {
		st.Upstream = up
	}
	if out, err := sys.Output(gitArgv(dir, "status", "--porcelain", "--untracked-files=no")...); err == nil && out != "" {
		st.Dirty = strings.Split(out, "\n")
	}
	return st
}

// ResolveHiveMCPDir is ${HIVE_MCP_DIR:-~/hive-mcp}.
func ResolveHiveMCPDir(sys UpdateSys) string {
	if d := sys.Getenv(HiveMCPDirEnv); d != "" {
		return expandPath(d)
	}
	home, _ := sys.Home()
	return filepath.Join(home, "hive-mcp")
}

// --- promote (pure) --------------------------------------------------------

// RepoUpdatePlan is the git work for one checkout.
type RepoUpdatePlan struct {
	Name     string
	Dir      string
	Upstream string
	Fetch    []string
	FFMerge  []string
}

// ErrRefused marks a checkout `hive update` will not touch.
var ErrRefused = errors.New("refused")

// PlanRepoUpdate decides, from observed state alone, whether a fast-forward
// is safe and which commands perform it. A refusal names how to proceed.
func PlanRepoUpdate(st RepoState) (RepoUpdatePlan, error) {
	p := RepoUpdatePlan{Name: st.Name, Dir: st.Dir, Upstream: st.Upstream}
	switch {
	case !st.Present:
		return p, fmt.Errorf("%w: %s: %s is not a git checkout; run 'hive setup' to install it", ErrRefused, st.Name, st.Dir)
	case len(st.Dirty) > 0:
		return p, fmt.Errorf("%w: %s: %s has local changes (%d path(s), e.g. %q); commit or stash them (git -C %s stash), then rerun 'hive update'",
			ErrRefused, st.Name, st.Dir, len(st.Dirty), strings.TrimSpace(st.Dirty[0]), st.Dir)
	case st.Branch == "" || st.Branch == "HEAD":
		return p, fmt.Errorf("%w: %s: %s is on a detached HEAD; check out its tracked branch (git -C %s switch main), then rerun 'hive update'",
			ErrRefused, st.Name, st.Dir, st.Dir)
	case st.Upstream == "":
		return p, fmt.Errorf("%w: %s: branch %s in %s tracks no remote branch; switch to the tracked branch (git -C %s switch main) or set one (git -C %s branch -u origin/%s), then rerun 'hive update'",
			ErrRefused, st.Name, st.Branch, st.Dir, st.Dir, st.Dir, st.Branch)
	}
	remote := st.Upstream
	if i := strings.Index(remote, "/"); i > 0 {
		remote = remote[:i]
	}
	p.Fetch = gitArgv(st.Dir, "fetch", remote)
	p.FFMerge = gitArgv(st.Dir, "merge", "--ff-only", "@{u}")
	return p, nil
}

// UpdatePlan is the whole of `hive update`.
type UpdatePlan struct {
	Repos []RepoUpdatePlan
	// Refusals are checkouts left alone, each with how to proceed.
	Refusals []error
	// Check is run last: bb-mcp verifies its anchor and registrations.
	Check []string
}

// UpdateTargets is the observed input to PlanUpdate.
type UpdateTargets struct {
	BBMCP RepoState
	// HiveMCP is consulted only with All.
	HiveMCP RepoState
	All     bool
}

// PlanUpdate plans every repo. bb-mcp comes first and is always part of the
// plan; hive-mcp joins with --all when `hive setup` cloned it, and is skipped
// silently when it is not there at all.
func PlanUpdate(t UpdateTargets) UpdatePlan {
	var up UpdatePlan
	add := func(st RepoState) {
		if p, err := PlanRepoUpdate(st); err != nil {
			up.Refusals = append(up.Refusals, err)
		} else {
			up.Repos = append(up.Repos, p)
		}
	}
	add(t.BBMCP)
	if t.All && t.HiveMCP.Present {
		add(t.HiveMCP)
	}
	if t.BBMCP.Present {
		up.Check = BBMCPSetupArgv(t.BBMCP.Dir, "--check")
	}
	return up
}

// DescribeUpdate renders the plan as the lines --dry-run prints.
func DescribeUpdate(up UpdatePlan) []string {
	var lines []string
	for _, r := range up.Repos {
		lines = append(lines,
			fmt.Sprintf("%s: fast-forward %s to %s", r.Name, r.Dir, r.Upstream),
			"  "+strings.Join(r.Fetch, " "),
			"  "+strings.Join(r.FFMerge, " "))
	}
	for _, e := range up.Refusals {
		lines = append(lines, "skip "+e.Error())
	}
	if len(up.Check) > 0 {
		lines = append(lines, "verify: "+strings.Join(up.Check, " "))
	}
	return lines
}

// --- boundary --------------------------------------------------------------

// UpdateOptions are the flags of `hive update`.
type UpdateOptions struct {
	DryRun bool
	All    bool
}

// ObserveUpdate collects the targets: bb-mcp through the anchor (falling back
// to ${BB_MCP_DIR:-~/bb-mcp}), hive-mcp at ${HIVE_MCP_DIR:-~/hive-mcp}.
func ObserveUpdate(sys UpdateSys, all bool) UpdateTargets {
	t := UpdateTargets{
		BBMCP: ObserveRepo(sys, "bb-mcp", ResolveBBMCPDir(sys.bbmcpSys())),
		All:   all,
	}
	if all {
		t.HiveMCP = ObserveRepo(sys, "hive-mcp", ResolveHiveMCPDir(sys))
	}
	return t
}

// RunUpdate executes `hive update`. Every refusal and failure is reported;
// the returned error is non-nil when anything was refused or failed.
func RunUpdate(sys UpdateSys, opts UpdateOptions) error {
	plan := PlanUpdate(ObserveUpdate(sys, opts.All))
	if opts.DryRun {
		sys.Say("dry run, nothing changes:")
		for _, l := range DescribeUpdate(plan) {
			sys.Say(l)
		}
		return nil
	}

	var failed []error
	failed = append(failed, plan.Refusals...)
	for _, e := range plan.Refusals {
		sys.Say(e.Error())
	}
	for _, r := range plan.Repos {
		sys.Say(fmt.Sprintf("%s: fetching and fast-forwarding %s to %s", r.Name, r.Dir, r.Upstream))
		if err := sys.Run(r.Fetch...); err != nil {
			failed = append(failed, fmt.Errorf("%s: %s failed: %w", r.Name, strings.Join(r.Fetch, " "), err))
			sys.Say(failed[len(failed)-1].Error())
			continue
		}
		if err := sys.Run(r.FFMerge...); err != nil {
			failed = append(failed, fmt.Errorf("%s: cannot fast-forward %s to %s (local commits diverge?); inspect with git -C %s log --oneline @{u}...HEAD: %w",
				r.Name, r.Dir, r.Upstream, r.Dir, err))
			sys.Say(failed[len(failed)-1].Error())
		}
	}
	if len(plan.Check) > 0 {
		sys.Say("verifying: " + strings.Join(plan.Check, " "))
		if err := sys.Run(plan.Check...); err != nil {
			failed = append(failed, fmt.Errorf("bb-mcp setup --check reports problems; fix with %s/bb-mcp setup: %w", plan.checkoutDir(), err))
			sys.Say(failed[len(failed)-1].Error())
		}
	}
	return errors.Join(failed...)
}

// checkoutDir is the bb-mcp checkout the check ran from, for the remedy line.
func (up UpdatePlan) checkoutDir() string {
	if len(up.Check) > 0 {
		return filepath.Dir(up.Check[0])
	}
	return ""
}
