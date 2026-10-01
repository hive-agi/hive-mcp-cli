package setup

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// --- pure plan ---------------------------------------------------------------

func cleanRepo(dir string) RepoState {
	return RepoState{Name: "bb-mcp", Dir: dir, Present: true, Branch: "main", Upstream: "origin/main"}
}

func TestPlanRepoUpdateFastForwardsATrackedCleanCheckout(t *testing.T) {
	p, err := PlanRepoUpdate(cleanRepo("/src/bb-mcp"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"git", "-C", "/src/bb-mcp", "fetch", "origin"}; !reflect.DeepEqual(p.Fetch, want) {
		t.Errorf("fetch %v, want %v", p.Fetch, want)
	}
	if want := []string{"git", "-C", "/src/bb-mcp", "merge", "--ff-only", "@{u}"}; !reflect.DeepEqual(p.FFMerge, want) {
		t.Errorf("merge %v, want %v", p.FFMerge, want)
	}
}

func TestPlanRepoUpdateFetchesTheUpstreamsRemote(t *testing.T) {
	st := cleanRepo("/r")
	st.Upstream = "upstream/release"
	p, _ := PlanRepoUpdate(st)
	if p.Fetch[len(p.Fetch)-1] != "upstream" {
		t.Errorf("fetched %v, want remote upstream", p.Fetch)
	}
}

func TestPlanRepoUpdateRefusals(t *testing.T) {
	cases := map[string]struct {
		mutate func(*RepoState)
		hint   string
	}{
		"missing":   {func(s *RepoState) { s.Present = false }, "hive setup"},
		"dirty":     {func(s *RepoState) { s.Dirty = []string{" M src/x.clj"} }, "stash"},
		"detached":  {func(s *RepoState) { s.Branch = "HEAD" }, "switch"},
		"untracked": {func(s *RepoState) { s.Upstream = "" }, "branch -u origin/main"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			st := cleanRepo("/r")
			c.mutate(&st)
			p, err := PlanRepoUpdate(st)
			if !errors.Is(err, ErrRefused) {
				t.Fatalf("want a refusal, got %v", err)
			}
			if !strings.Contains(err.Error(), c.hint) {
				t.Errorf("refusal %q does not say how to proceed (%q)", err, c.hint)
			}
			if p.Fetch != nil || p.FFMerge != nil {
				t.Errorf("a refused plan carries commands: %+v", p)
			}
		})
	}
}

func TestPlanUpdateLeavesHiveMCPOutWithoutAll(t *testing.T) {
	up := PlanUpdate(UpdateTargets{
		BBMCP:   cleanRepo("/b"),
		HiveMCP: RepoState{Name: "hive-mcp", Dir: "/h", Present: true, Branch: "main", Upstream: "origin/main"},
	})
	if len(up.Repos) != 1 || up.Repos[0].Dir != "/b" {
		t.Errorf("repos %+v", up.Repos)
	}
	if want := []string{"/b/bb-mcp", "setup", "--check"}; !reflect.DeepEqual(up.Check, want) {
		t.Errorf("check %v, want %v", up.Check, want)
	}
}

func TestPlanUpdateAllAddsHiveMCPOnlyWhenCloned(t *testing.T) {
	hm := RepoState{Name: "hive-mcp", Dir: "/h", Present: true, Branch: "main", Upstream: "origin/main"}
	up := PlanUpdate(UpdateTargets{BBMCP: cleanRepo("/b"), HiveMCP: hm, All: true})
	if len(up.Repos) != 2 || up.Repos[1].Dir != "/h" {
		t.Errorf("repos %+v", up.Repos)
	}
	up = PlanUpdate(UpdateTargets{BBMCP: cleanRepo("/b"), HiveMCP: RepoState{Name: "hive-mcp", Dir: "/h"}, All: true})
	if len(up.Repos) != 1 || len(up.Refusals) != 0 {
		t.Errorf("an absent hive-mcp must be skipped silently: %+v", up)
	}
}

func TestPlanUpdateRefusalKeepsTheOtherRepoAndTheCheck(t *testing.T) {
	dirty := cleanRepo("/b")
	dirty.Dirty = []string{"?? new"}
	hm := RepoState{Name: "hive-mcp", Dir: "/h", Present: true, Branch: "main", Upstream: "origin/main"}
	up := PlanUpdate(UpdateTargets{BBMCP: dirty, HiveMCP: hm, All: true})
	if len(up.Repos) != 1 || up.Repos[0].Name != "hive-mcp" || len(up.Refusals) != 1 || up.Check == nil {
		t.Errorf("plan %+v", up)
	}
}

func TestDescribeUpdateListsEveryCommand(t *testing.T) {
	dirty := cleanRepo("/h")
	dirty.Name, dirty.Dirty = "hive-mcp", []string{" M x"}
	all := strings.Join(DescribeUpdate(PlanUpdate(UpdateTargets{BBMCP: cleanRepo("/b"), HiveMCP: dirty, All: true})), "\n")
	for _, want := range []string{"git -C /b fetch origin", "git -C /b merge --ff-only @{u}", "skip refused: hive-mcp", "verify: /b/bb-mcp setup --check"} {
		if !strings.Contains(all, want) {
			t.Errorf("dry run misses %q:\n%s", want, all)
		}
	}
}

// --- collect + boundary, against a temp HOME and a fake git -----------------

type fakeGit struct {
	out   map[string]string // keyed by argv joined with spaces
	fail  map[string]error
	calls [][]string
	said  []string
}

func (f *fakeGit) output(argv ...string) (string, error) {
	k := strings.Join(argv, " ")
	if err, ok := f.fail[k]; ok {
		return "", err
	}
	return f.out[k], nil
}

func (f *fakeGit) run(argv ...string) error {
	f.calls = append(f.calls, argv)
	return f.fail[strings.Join(argv, " ")]
}

func updateSys(home string, env map[string]string, g *fakeGit) UpdateSys {
	sys := DefaultUpdateSys()
	sys.Home = func() (string, error) { return home, nil }
	sys.Getenv = func(k string) string { return env[k] }
	sys.Output = g.output
	sys.Run = g.run
	sys.Say = func(s string) { g.said = append(g.said, s) }
	return sys
}

func gitRepo(t *testing.T, dir string, g *fakeGit, branch, upstream, status string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	g.out["git -C "+dir+" rev-parse --abbrev-ref HEAD"] = branch
	k := "git -C " + dir + " rev-parse --abbrev-ref --symbolic-full-name @{u}"
	if upstream == "" {
		g.fail[k] = errors.New("no upstream")
	} else {
		g.out[k] = upstream
	}
	g.out["git -C "+dir+" status --porcelain --untracked-files=no"] = status
}

func newFakeGit() *fakeGit {
	return &fakeGit{out: map[string]string{}, fail: map[string]error{}}
}

// anchoredCheckout publishes a bb-mcp checkout at the anchor and returns its
// real path (the anchor is resolved through symlinks).
func anchoredCheckout(t *testing.T, home string) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "bb-mcp")
	makeCheckout(t, src)
	publishAnchor(t, home, src)
	real, _ := filepath.EvalSymlinks(src)
	return real
}

func TestRunUpdateResolvesBBMCPThroughTheAnchor(t *testing.T) {
	home := t.TempDir()
	src := anchoredCheckout(t, home)
	g := newFakeGit()
	gitRepo(t, src, g, "main", "origin/main", "")

	if err := RunUpdate(updateSys(home, nil, g), UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"git", "-C", src, "fetch", "origin"},
		{"git", "-C", src, "merge", "--ff-only", "@{u}"},
		{filepath.Join(src, "bb-mcp"), "setup", "--check"},
	}
	if !reflect.DeepEqual(g.calls, want) {
		t.Errorf("calls\n%v\nwant\n%v", g.calls, want)
	}
}

func TestRunUpdateDryRunRunsNothing(t *testing.T) {
	home := t.TempDir()
	src := anchoredCheckout(t, home)
	g := newFakeGit()
	gitRepo(t, src, g, "main", "origin/main", "")

	if err := RunUpdate(updateSys(home, nil, g), UpdateOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 0 {
		t.Errorf("dry run executed %v", g.calls)
	}
	if all := strings.Join(g.said, "\n"); !strings.Contains(all, "merge --ff-only") {
		t.Errorf("dry run did not print the plan:\n%s", all)
	}
}

func TestRunUpdateRefusesADirtyCheckoutButStillChecks(t *testing.T) {
	home := t.TempDir()
	src := anchoredCheckout(t, home)
	g := newFakeGit()
	gitRepo(t, src, g, "main", "origin/main", " M src/bb_mcp/core.clj")

	err := RunUpdate(updateSys(home, nil, g), UpdateOptions{})
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("want a refusal, got %v", err)
	}
	if len(g.calls) != 1 || g.calls[0][1] != "setup" {
		t.Errorf("a dirty checkout must not be fetched or merged: %v", g.calls)
	}
}

func TestRunUpdateReportsAFailedFastForward(t *testing.T) {
	home := t.TempDir()
	src := anchoredCheckout(t, home)
	g := newFakeGit()
	gitRepo(t, src, g, "main", "origin/main", "")
	g.fail["git -C "+src+" merge --ff-only @{u}"] = errors.New("exit 128")

	err := RunUpdate(updateSys(home, nil, g), UpdateOptions{})
	if err == nil || !strings.Contains(err.Error(), "cannot fast-forward") {
		t.Fatalf("got %v", err)
	}
}

func TestRunUpdateAllUpdatesHiveMCPFromItsEnvDir(t *testing.T) {
	home := t.TempDir()
	src := anchoredCheckout(t, home)
	hm := filepath.Join(t.TempDir(), "hive-mcp")
	g := newFakeGit()
	gitRepo(t, src, g, "main", "origin/main", "")
	gitRepo(t, hm, g, "staging", "origin/staging", "")

	if err := RunUpdate(updateSys(home, map[string]string{HiveMCPDirEnv: hm}, g), UpdateOptions{All: true}); err != nil {
		t.Fatal(err)
	}
	var sawHM bool
	for _, c := range g.calls {
		if reflect.DeepEqual(c, []string{"git", "-C", hm, "merge", "--ff-only", "@{u}"}) {
			sawHM = true
		}
	}
	if !sawHM {
		t.Errorf("hive-mcp was not fast-forwarded: %v", g.calls)
	}
}
