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

func TestPlanReusesTheCheckoutTheAnchorResolvesTo(t *testing.T) {
	p, err := PlanBBMCP(BBMCPObserved{
		Anchor:         "/h/.local/share/hive-mcp/bb-mcp",
		AnchorCheckout: "/src/bb-mcp",
		Target:         "/h/bb-mcp",
	})
	if err != nil || p.Clone || p.Checkout != "/src/bb-mcp" {
		t.Fatalf("got %+v, %v", p, err)
	}
	want := []string{"/src/bb-mcp/bb-mcp", "setup", "--client", "all"}
	if !reflect.DeepEqual(p.Setup, want) {
		t.Errorf("setup argv %v, want %v", p.Setup, want)
	}
}

func TestPlanReusesAnExistingCheckoutAtTheTarget(t *testing.T) {
	p, err := PlanBBMCP(BBMCPObserved{Target: "/h/bb-mcp", TargetExists: true, TargetIsCheckout: true})
	if err != nil || p.Clone || p.Checkout != "/h/bb-mcp" {
		t.Fatalf("got %+v, %v", p, err)
	}
}

func TestPlanClonesWhenNothingIsThere(t *testing.T) {
	p, err := PlanBBMCP(BBMCPObserved{Target: "/h/bb-mcp"})
	if err != nil || !p.Clone || p.Checkout != "/h/bb-mcp" {
		t.Fatalf("got %+v, %v", p, err)
	}
	if p.Setup[0] != "/h/bb-mcp/bb-mcp" {
		t.Errorf("setup must run from the fresh clone: %v", p.Setup)
	}
}

func TestPlanRefusesATargetThatIsNotACheckout(t *testing.T) {
	_, err := PlanBBMCP(BBMCPObserved{Target: "/h/bb-mcp", TargetExists: true})
	if err == nil || !strings.Contains(err.Error(), BBMCPDirEnv) {
		t.Fatalf("want a refusal naming %s, got %v", BBMCPDirEnv, err)
	}
}

func TestPlanReportsLegacyEntries(t *testing.T) {
	p, _ := PlanBBMCP(BBMCPObserved{
		Target: "/h/bb-mcp",
		Legacy: []LegacyEntry{
			{Scope: "user", Command: "/h/hive-mcp/bin/hive-mcp-foss"},
			{Scope: "local", Project: "/h/app", Command: "/h/hive-mcp/bin/hive-mcp-foss"},
		},
	})
	all := strings.Join(p.Notes, "\n")
	for _, want := range []string{"replaced by bb-mcp setup", `cd "/h/app" && claude mcp remove --scope local hive`} {
		if !strings.Contains(all, want) {
			t.Errorf("notes miss %q:\n%s", want, all)
		}
	}
}

func TestLegacyHiveEntriesFindsUserAndLocalScopes(t *testing.T) {
	raw := []byte(`{
	  "mcpServers": {"hive": {"command": "/u/hive-mcp/bin/hive-mcp-foss"},
	                 "other": {"command": "x"}},
	  "projects": {
	    "/b": {"mcpServers": {"hive": {"command": "/u/hive-mcp/bin/hive-mcp-foss"}}},
	    "/a": {"mcpServers": {"hive": {"command": "/u/.local/share/hive-mcp/bb-mcp/start-bb-mcp.sh"}}},
	    "/c": {"mcpServers": {}}
	  }}`)
	got := LegacyHiveEntries(raw)
	want := []LegacyEntry{
		{Scope: "user", Command: "/u/hive-mcp/bin/hive-mcp-foss"},
		{Scope: "local", Project: "/b", Command: "/u/hive-mcp/bin/hive-mcp-foss"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestLegacyHiveEntriesIgnoresBbMcpAndGarbage(t *testing.T) {
	if got := LegacyHiveEntries([]byte(`{"mcpServers":{"hive":{"command":"/x/start-bb-mcp.sh"}}}`)); len(got) != 0 {
		t.Errorf("bb-mcp entry flagged as legacy: %+v", got)
	}
	if got := LegacyHiveEntries([]byte(`not json`)); got != nil {
		t.Errorf("garbage yielded %+v", got)
	}
}

// --- collect + boundary, against a temp HOME and a fake runner ---------------

type fakeRunner struct {
	calls [][]string
	fail  map[string]error // keyed by argv joined with spaces
	// onClone simulates git clone producing a checkout.
	onClone func(dir string)
}

func (f *fakeRunner) run(argv ...string) error {
	f.calls = append(f.calls, argv)
	if len(argv) >= 4 && argv[0] == "git" && argv[1] == "clone" && f.onClone != nil {
		f.onClone(argv[3])
	}
	return f.fail[strings.Join(argv, " ")]
}

func testSys(t *testing.T, home string, env map[string]string, r *fakeRunner) *BBMCPSys {
	t.Helper()
	sys := DefaultBBMCPSys()
	sys.Home = func() (string, error) { return home, nil }
	sys.Getenv = func(k string) string { return env[k] }
	sys.Run = r.run
	sys.Say = func(string) {}
	return &sys
}

func makeCheckout(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"bb-mcp", "start-bb-mcp.sh"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func publishAnchor(t *testing.T, home, checkout string) string {
	t.Helper()
	anchor := BBMCPAnchor(home)
	if err := os.MkdirAll(filepath.Dir(anchor), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(checkout, anchor); err != nil {
		t.Fatal(err)
	}
	return anchor
}

func TestObserveResolvesTheAnchorToTheRealCheckout(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(t.TempDir(), "elsewhere", "bb-mcp")
	makeCheckout(t, src)
	publishAnchor(t, home, src)

	o := ObserveBBMCP(*testSys(t, home, nil, &fakeRunner{}))
	real, _ := filepath.EvalSymlinks(src)
	if o.AnchorCheckout != real {
		t.Errorf("AnchorCheckout = %q, want %q", o.AnchorCheckout, real)
	}
	if o.Target != filepath.Join(home, "bb-mcp") || o.TargetExists {
		t.Errorf("target: %+v", o)
	}
}

func TestObserveIgnoresAnAnchorThatIsNotACheckout(t *testing.T) {
	home := t.TempDir()
	empty := t.TempDir()
	publishAnchor(t, home, empty)
	if o := ObserveBBMCP(*testSys(t, home, nil, &fakeRunner{})); o.AnchorCheckout != "" {
		t.Errorf("an empty dir counted as a checkout: %+v", o)
	}
}

func TestObserveHonoursBBMCPDir(t *testing.T) {
	home := t.TempDir()
	custom := filepath.Join(t.TempDir(), "mine")
	makeCheckout(t, custom)
	o := ObserveBBMCP(*testSys(t, home, map[string]string{BBMCPDirEnv: custom}, &fakeRunner{}))
	if o.Target != custom || !o.TargetIsCheckout {
		t.Errorf("got %+v", o)
	}
}

func TestStepClonesThenRunsSetupFromTheClone(t *testing.T) {
	home := t.TempDir()
	r := &fakeRunner{onClone: func(dir string) { makeCheckout(t, dir) }}
	step := &BBMCPStep{Sys: testSys(t, home, nil, r)}

	if err := step.Run(); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "bb-mcp")
	want := [][]string{
		{"git", "clone", BBMCPRepoURL, target},
		{filepath.Join(target, "bb-mcp"), "setup", "--client", "all"},
	}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls %v\nwant  %v", r.calls, want)
	}
}

func TestStepReusesTheAnchorAndNeverClones(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(t.TempDir(), "bb-mcp")
	makeCheckout(t, src)
	publishAnchor(t, home, src)
	real, _ := filepath.EvalSymlinks(src)

	r := &fakeRunner{}
	if err := (&BBMCPStep{Sys: testSys(t, home, nil, r)}).Run(); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{filepath.Join(real, "bb-mcp"), "setup", "--client", "all"}}
	if !reflect.DeepEqual(r.calls, want) {
		t.Errorf("calls %v, want %v", r.calls, want)
	}
}

func TestStepSurfacesASetupFailure(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "bb-mcp")
	makeCheckout(t, target)
	argv := strings.Join(BBMCPSetupArgv(target, "--client", "all"), " ")
	r := &fakeRunner{fail: map[string]error{argv: errors.New("exit 1")}}
	if err := (&BBMCPStep{Sys: testSys(t, home, nil, r)}).Run(); err == nil {
		t.Fatal("a failed bb-mcp setup must fail the step")
	}
}

func TestCheckDelegatesToBbMcpSetupCheck(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(t.TempDir(), "bb-mcp")
	makeCheckout(t, src)
	publishAnchor(t, home, src)
	real, _ := filepath.EvalSymlinks(src)
	checkArgv := strings.Join(BBMCPSetupArgv(real, "--check"), " ")

	ok := &fakeRunner{}
	if done, _ := (&BBMCPStep{Sys: testSys(t, home, nil, ok)}).Check(); !done {
		t.Error("a passing --check must mark the step done")
	}
	if len(ok.calls) != 1 || strings.Join(ok.calls[0], " ") != checkArgv {
		t.Errorf("calls %v, want [%s]", ok.calls, checkArgv)
	}

	bad := &fakeRunner{fail: map[string]error{checkArgv: errors.New("exit 1")}}
	if done, _ := (&BBMCPStep{Sys: testSys(t, home, nil, bad)}).Check(); done {
		t.Error("a failing --check must leave the step to run")
	}
}

func TestCheckRunsTheMigrationWhileALegacyEntryRemains(t *testing.T) {
	home := t.TempDir()
	src := filepath.Join(t.TempDir(), "bb-mcp")
	makeCheckout(t, src)
	publishAnchor(t, home, src)
	os.WriteFile(filepath.Join(home, ".claude.json"),
		[]byte(`{"mcpServers":{"hive":{"command":"/x/bin/hive-mcp-foss"}}}`), 0o600)

	r := &fakeRunner{}
	if done, _ := (&BBMCPStep{Sys: testSys(t, home, nil, r)}).Check(); done {
		t.Error("a legacy hive-mcp-foss entry must make the step run")
	}
	if len(r.calls) != 0 {
		t.Errorf("no --check needed when the answer is already no: %v", r.calls)
	}
}

func TestCheckWithoutAnAnchorRunsTheStep(t *testing.T) {
	r := &fakeRunner{}
	if done, _ := (&BBMCPStep{Sys: testSys(t, t.TempDir(), nil, r)}).Check(); done {
		t.Error("no anchor means not done")
	}
}

func TestDetachedBackendKeepsStdinOpen(t *testing.T) {
	argv := DetachedBackendArgv("/x/bin/hive-mcp-foss")
	if argv[0] != "sh" || !strings.Contains(argv[2], "tail -f /dev/null |") || argv[3] != "/x/bin/hive-mcp-foss" {
		t.Errorf("got %v", argv)
	}
}

func TestBackendAddrHonoursHIVE_NREPL_PORT(t *testing.T) {
	env := map[string]string{}
	get := func(k string) string { return env[k] }
	if got := BackendNreplAddr(get); got != "127.0.0.1:7910" {
		t.Errorf("default %s", got)
	}
	env["HIVE_NREPL_PORT"] = "7950"
	if got := BackendNreplAddr(get); got != "127.0.0.1:7950" {
		t.Errorf("override %s", got)
	}
}
