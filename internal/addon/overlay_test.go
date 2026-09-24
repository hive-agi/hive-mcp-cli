package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddToOverlayCreatesAFileTheLauncherCanMerge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.deps.edn")
	res, _, err := AddToOverlay(path, "io.github.hive-agi/hive-carto", "0.4.623", "https://store.example/")
	if err != nil || res != OverlayCreated {
		t.Fatalf("got %v, %v", res, err)
	}
	b, _ := os.ReadFile(path)
	body := string(b)
	for _, want := range []string{
		`:mvn/repos {"hive-store" {:url "https://store.example/maven"}}`,
		`:deps {io.github.hive-agi/hive-carto {:mvn/version "0.4.623"}}}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in\n%s", want, body)
		}
	}
	// -Sdeps reads an argument that does not start with "{" as a file path.
	if !strings.HasPrefix(body, "{") {
		t.Errorf("overlay must start with {, got %q", body[:20])
	}
}

func TestAddToOverlayRecognisesAnExistingEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.deps.edn")
	os.WriteFile(path, []byte(`{:deps {io.github.hive-agi/hive-carto {:mvn/version "1"}}}`), 0o644)
	if res, _, _ := AddToOverlay(path, "io.github.hive-agi/hive-carto", "", "https://s"); res != OverlayPresent {
		t.Errorf("got %v, want OverlayPresent", res)
	}
}

func TestAddToOverlayNeverRewritesAUsersFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.deps.edn")
	orig := ";; mine\n{:deps {io.github.hive-agi/hive-carto-flow {:mvn/version \"1\"}}}\n"
	os.WriteFile(path, []byte(orig), 0o644)
	res, snippet, _ := AddToOverlay(path, "io.github.hive-agi/hive-carto", "", "https://s")
	if res != OverlayManual {
		t.Errorf("a longer coordinate must not count as present; got %v", res)
	}
	if b, _ := os.ReadFile(path); string(b) != orig {
		t.Error("the user's overlay was modified")
	}
	if !strings.Contains(snippet, `io.github.hive-agi/hive-carto {:mvn/version "RELEASE"}`) {
		t.Errorf("snippet: %s", snippet)
	}
}
