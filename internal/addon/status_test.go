package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestArgsLayerHiveAddonOverTheProjectsOwnDeps(t *testing.T) {
	args := Args("")
	if args[0] != "clojure" {
		t.Fatalf("argv[0] is %q", args[0])
	}
	joined := strings.Join(args, " ")
	has(t, joined, "-Sdeps")
	has(t, joined, `io.github.hive-agi/hive-addon {:mvn/version "`+AddonVersion+`"}`)
	// -M and not -T: the project's own classpath has to be present, because the
	// addons being discovered are ON it.
	has(t, joined, "-M")
	has(t, joined, "-e")
}

func TestArgsAcceptAPinnedVersion(t *testing.T) {
	has(t, strings.Join(Args("0.9.1"), " "), `{:mvn/version "0.9.1"}`)
}

func TestTheProgramAsksTheMounterAndDecidesNothingItself(t *testing.T) {
	// dry-run applies the licence gate and constructs nothing.
	has(t, Program, "b/dry-run")
	has(t, Program, "e/installed-gate")
	// No entitlement arithmetic here: the CLI must not form a second opinion
	// about what a licence permits.
	hasNot(t, Program, "expires")
	hasNot(t, Program, "entitles")
	hasNot(t, Program, "verify")
	// And it must not MOUNT anything: status is a question, not an action.
	hasNot(t, Program, "b/mount!")
	hasNot(t, Program, "teardown")
}

func TestTheProgramReportsUnreadableManifestsRatherThanDroppingThem(t *testing.T) {
	// discover-specs returns {:specs .. :errors ..} and a bad manifest becomes
	// an :errors entry. A status that printed only :specs would show a shorter
	// list and call it success.
	has(t, Program, ":errors")
	has(t, Program, "Unreadable manifests")
}

func TestTheProgramNamesTheGateAndExplainsTheClosedDefault(t *testing.T) {
	has(t, Program, "Gate: %s")
	has(t, Program, ":closed")
	has(t, Program, "no licence is installed")
}

func TestIsProjectRecognisesTheThreeProjectFiles(t *testing.T) {
	for _, name := range []string{"deps.edn", "bb.edn", "project.clj"} {
		dir := t.TempDir()
		if IsProject(dir) {
			t.Fatalf("an empty dir is not a project")
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if !IsProject(dir) {
			t.Fatalf("%s should make it a project", name)
		}
	}
}

func TestIsProjectToleratesATrailingSlash(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "deps.edn"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IsProject(dir + "/") {
		t.Fatal("a trailing slash must not change the answer")
	}
}
