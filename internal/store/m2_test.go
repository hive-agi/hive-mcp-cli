package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertServerEmptyWritesWholeDocument(t *testing.T) {
	got, err := UpsertServer("", RepoID, "hv_live_abc")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<settings>", "<id>hive-store</id>", "<username>hv_live_abc</username>", "<password>hv_live_abc</password>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}

func TestUpsertServerKeepsOtherServersAndReplacesOurs(t *testing.T) {
	doc := `<settings>
  <!-- mine -->
  <servers>
    <server>
      <id>clojars</id>
      <username>me</username>
      <password>secret</password>
    </server>
    <server>
      <id>hive-store</id>
      <username>hv_live_old</username>
      <password>typo</password>
    </server>
  </servers>
</settings>
`
	got, err := UpsertServer(doc, RepoID, "hv_live_new")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "hv_live_old") || strings.Contains(got, "typo") {
		t.Errorf("old credential survived:\n%s", got)
	}
	if strings.Count(got, "<id>hive-store</id>") != 1 {
		t.Errorf("want exactly one hive-store server:\n%s", got)
	}
	for _, keep := range []string{"<!-- mine -->", "<id>clojars</id>", "<password>secret</password>"} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %q:\n%s", keep, got)
		}
	}
}

func TestUpsertServerAddsServersSection(t *testing.T) {
	doc := "<settings>\n  <localRepository>/x</localRepository>\n</settings>\n"
	got, err := UpsertServer(doc, RepoID, "hv_live_x")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "<servers>") || !strings.Contains(got, "<localRepository>/x</localRepository>") {
		t.Errorf("bad merge:\n%s", got)
	}
}

func TestUpsertServerIsIdempotent(t *testing.T) {
	once, _ := UpsertServer("", RepoID, "hv_live_x")
	twice, _ := UpsertServer(once, RepoID, "hv_live_x")
	if once != twice {
		t.Errorf("second upsert changed the file:\n%s\n---\n%s", once, twice)
	}
}

func TestUpsertServerRefusesUnreadableFile(t *testing.T) {
	if _, err := UpsertServer("not xml at all", RepoID, "t"); err == nil {
		t.Error("want an error for a file with no </settings>")
	}
}

func TestUpsertServerEscapesToken(t *testing.T) {
	got, _ := UpsertServer("", RepoID, "a<b&c")
	if !strings.Contains(got, "a&lt;b&amp;c") {
		t.Errorf("token not escaped:\n%s", got)
	}
}

func TestRemoveServerOnlyRemovesOurToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.xml")
	doc, _ := UpsertServer("", "clojars", "mine")
	doc, _ = UpsertServer(doc, RepoID, "hv_live_ours")
	os.WriteFile(path, []byte(doc), 0o600)

	if removed, _ := RemoveServerIfToken(path, RepoID, "hv_live_someone_else"); removed {
		t.Fatal("removed a block holding a different token")
	}
	removed, err := RemoveServerIfToken(path, RepoID, "hv_live_ours")
	if err != nil || !removed {
		t.Fatalf("removed=%v err=%v", removed, err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), RepoID) || !strings.Contains(string(b), "<id>clojars</id>") {
		t.Errorf("after removal:\n%s", b)
	}
}

func TestWriteSettingsBacksUpAndIsPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".m2", "settings.xml")
	if _, err := WriteSettings(path, RepoID, "hv_live_1"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want 0600", info.Mode().Perm())
	}
	backup, err := WriteSettings(path, RepoID, "hv_live_2")
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("changing an existing file must leave a backup")
	}
	if b, _ := os.ReadFile(backup); !strings.Contains(string(b), "hv_live_1") {
		t.Error("backup does not hold the previous file")
	}
	if backup2, _ := WriteSettings(path, RepoID, "hv_live_2"); backup2 != "" {
		t.Error("an unchanged file must not be rewritten or backed up")
	}
}
