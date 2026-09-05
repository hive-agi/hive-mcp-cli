package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func has(t *testing.T, s, sub string) {
	t.Helper()
	if !strings.Contains(s, sub) {
		t.Fatalf("expected to find %q in:\n%s", sub, s)
	}
}

func hasNot(t *testing.T, s, sub string) {
	t.Helper()
	if strings.Contains(s, sub) {
		t.Fatalf("did not expect %q in:\n%s", sub, s)
	}
}

func TestNsPathMungesHyphensAsTheLoaderExpects(t *testing.T) {
	if got, want := NsPath("acme.rank"), filepath.Join("acme", "rank.clj"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got, want := NsPath("acme.my-addon"), filepath.Join("acme", "my_addon.clj"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestManifestNameIsFlat(t *testing.T) {
	if got, want := ManifestName("acme.rank"), "acme-rank.edn"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAStandaloneManifestDeclaresNoDependency(t *testing.T) {
	m := Manifest(Spec{ID: "acme.rank", Capabilities: []string{"acme/rank"}})
	has(t, m, `:addon/id "acme.rank"`)
	has(t, m, `:addon/init-ns "acme.rank"`)
	has(t, m, `:addon/init-fn "addon-ctor"`)
	has(t, m, `:addon/config {:addon/id "acme.rank"}`)
	has(t, m, `:addon/capabilities #{:acme/rank}`)
	hasNot(t, m, ":addon/dependencies")
	// :foss and not :proprietary: a gated spec refuses to mount until a licence
	// gate permits it, which would lock an author out of their own addon.
	has(t, m, ":addon/trust-class :foss")
}

func TestTheConfigCarriesTheIDTheConstructorReads(t *testing.T) {
	// The default config resolver hands a constructor :addon/config and NOT the
	// spec, so an id that is not in the config arrives as nil, the addon
	// registers under nil, and init! then finds nothing to initialize.
	m := Manifest(Spec{ID: "acme.rank"})
	has(t, m, `:addon/config {:addon/id "acme.rank"}`)
	has(t, m, `:addon/id "acme.rank"`)
	src := Source(Spec{ID: "acme.rank"})
	has(t, src, "(:addon/id config)")
}

func TestAnExtensionDeclaresItsInnerAddonAsADependency(t *testing.T) {
	s := Spec{ID: "acme.ranked-carto", Extends: "hive.carto"}
	m := Manifest(s)
	has(t, m, `:addon/dependencies #{"hive.carto"}`)

	src := Source(s)
	has(t, src, `(get (:mount/dependencies config) "hive.carto")`)
	has(t, src, "(defrecord Addon [id inner]")
	// The vendor addon is composed, never edited: nothing in a generated
	// extension reaches into the other namespace.
	hasNot(t, src, "(:require [hive.carto")
	hasNot(t, src, "alter-var-root")
}

func TestAnExtensionIsToldItsInnerAddonCanBeAbsent(t *testing.T) {
	src := Source(Spec{ID: "acme.ext", Extends: "hive.carto"})
	has(t, src, "nil when hive.carto did not mount")
	has(t, src, "Your addon still mounts")
}

func TestGeneratedSourceImplementsEveryIAddonMethod(t *testing.T) {
	src := Source(Spec{ID: "acme.rank"})
	for _, m := range []string{
		"(addon-id [_] id)", "(addon-type [_] :native)", "(capabilities [_]",
		"(initialize! [_ _config]", "(shutdown! [_] nil)", "(tools [_] [])",
		"(schema-extensions [_] [])", "(health [_] {:status :ok})",
		"(excluded-tools [_] #{})", "(hooks [_] {})",
	} {
		has(t, src, m)
	}
	// A partial defrecord compiles and then throws AbstractMethodError at the
	// first call the host makes, which is at mount time on the user's machine.
	has(t, src, "proto/IAddon")
}

func TestFilesGoWhereTheClasspathScannerLooks(t *testing.T) {
	files := Files(Spec{ID: "acme.rank"})
	manifest := filepath.Join("resources", "META-INF", "hive-addons", "acme-rank.edn")
	if _, ok := files[manifest]; !ok {
		t.Fatalf("no manifest at %s; got %v", manifest, keys(files))
	}
	if _, ok := files[filepath.Join("src", "acme", "rank.clj")]; !ok {
		t.Fatalf("no source; got %v", keys(files))
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestWriteCreatesBothFilesUnderTheRoot(t *testing.T) {
	root := t.TempDir()
	written, err := Write(root, Spec{ID: "acme.rank"})
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 {
		t.Fatalf("wrote %v", written)
	}
	for _, rel := range written {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("%s was reported written but is not there: %v", rel, err)
		}
	}
}

func TestWriteRefusesToOverwriteAndTouchesNothing(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src", "acme", "rank.clj")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte(";; a year of my work\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Write(root, Spec{ID: "acme.rank"}); err == nil {
		t.Fatal("expected a refusal")
	}
	body, _ := os.ReadFile(src)
	if string(body) != ";; a year of my work\n" {
		t.Fatalf("the existing file was modified: %q", body)
	}
	// The refusal is checked for EVERY file before ANY is written, so a partial
	// scaffold cannot be left behind.
	manifest := filepath.Join(root, "resources", "META-INF", "hive-addons", "acme-rank.edn")
	if _, err := os.Stat(manifest); err == nil {
		t.Fatal("a file was written despite the refusal")
	}
}

func TestCoordinateNamesTheRegistryAndWhereTheTokenGoes(t *testing.T) {
	out := Coordinate("io.github.hive-agi/hive-carto", "0.1.396", "https://hive-mcp.com/")
	has(t, out, `io.github.hive-agi/hive-carto {:mvn/version "0.1.396"}`)
	has(t, out, `"hive-store" {:url "https://hive-mcp.com/maven"}`)
	has(t, out, "~/.m2/settings.xml")
}
