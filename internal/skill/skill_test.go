package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hive-agi/hive-mcp-cli/internal/store"
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

func sample() store.Addon {
	return store.Addon{
		ID:           "io.github.hive-agi:hive-ingestor",
		Name:         "hive-ingestor",
		Blurb:        "Documents become memory: PDF, Markdown and DOCX chunked and embedded.",
		Coordinate:   "io.github.hive-agi/hive-ingestor",
		Status:       "available",
		Kind:         "addon",
		Version:      "2.3.1",
		Tags:         []string{"knowledge"},
		Highlights:   []string{"Sources register themselves"},
		Capabilities: []string{"tools", "ingestion"},
		Requires:     []string{"io.github.hive-agi:hive-knowledge"},
		RequiredBy:   []string{"io.github.hive-agi:hive-ingestor-rfc"},
		ExtensionPoints: []store.ExtensionPoint{
			{
				Capability:  "sources",
				Summary:     "A corpus the ingestor can pull documents from.",
				Port:        "hive-ingestor.source.protocol/ISource",
				Registry:    "hive-ingestor.source.registry/register-source!",
				Cardinality: "many",
				FilledBy:    []string{"io.github.hive-agi:hive-ingestor-web"},
			},
			{
				Capability:  "parser-rules",
				Summary:     "A rule that decides how one document shape is parsed.",
				Cardinality: "many",
				FilledBy:    []string{},
			},
		},
	}
}

func TestSkillNameTakesTheArtifactHalfOfTheId(t *testing.T) {
	for id, want := range map[string]string{
		"io.github.hive-agi:hive-carto": "hive-carto",
		"io.github.hive-agi/hive-carto": "hive-carto",
		"hive-carto":                    "hive-carto",
	} {
		if got := SkillName(id); got != want {
			t.Fatalf("SkillName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestFrontmatterQuotesABlurbCarryingAColon(t *testing.T) {
	// The blurb is prose and YAML would end the key at a bare colon, so an
	// unquoted description silently truncates the skill's whole trigger set.
	fm := Frontmatter(sample())
	has(t, fm, `description: "Documents become memory: PDF`)
	has(t, fm, "name: hive-ingestor\n")
}

func TestFrontmatterTriggersOnTheAddonsOwnVocabulary(t *testing.T) {
	fm := Frontmatter(sample())
	has(t, fm, "Triggers on:")
	has(t, fm, "hive-ingestor")
	has(t, fm, "knowledge")
	has(t, fm, "ingestion")
}

func TestPreviewIsStatedAsARegistryFactNotAQualityJudgement(t *testing.T) {
	a := sample()
	a.Status = "preview"
	body := Body(a)
	has(t, body, "is PREVIEW")
	has(t, body, "does not resolve yet")
}

func TestBodyCarriesTheGraphBothWays(t *testing.T) {
	body := Body(sample())
	has(t, body, "## Addons it needs")
	has(t, body, "`io.github.hive-agi:hive-knowledge`")
	has(t, body, "## Addons built on it")
	has(t, body, "`io.github.hive-agi:hive-ingestor-rfc`")
}

func TestAVacantSeamSaysSoRatherThanLookingFilled(t *testing.T) {
	// The whole reason the wire sends filledBy EMPTY rather than absent: a
	// reader asking "could I write one" needs the empty case stated.
	body := Body(sample())
	has(t, body, "### `parser-rules`")
	has(t, body, "Filled by: NOTHING yet")
	has(t, body, "### `sources`")
	has(t, body, "Filled by: `io.github.hive-agi:hive-ingestor-web`")
}

func TestExtensionSectionNamesThePortAndTheRegistrar(t *testing.T) {
	body := Body(sample())
	has(t, body, "`hive-ingestor.source.protocol/ISource`")
	has(t, body, "`hive-ingestor.source.registry/register-source!`")
	has(t, body, "hive addon new --extends io.github.hive-agi:hive-ingestor")
}

func TestAnAddonWithNoSeamsGetsNoExtendingSection(t *testing.T) {
	a := sample()
	a.ExtensionPoints = nil
	hasNot(t, Body(a), "## Extending it")
}

func TestTheSkillDoesNotInventToolInvocations(t *testing.T) {
	// A skill that guesses a command teaches a subscriber to paste something
	// that does not run, which is the defect this renderer exists to avoid.
	body := Body(sample())
	has(t, body, "does NOT\ninvent tool invocations")
}

func TestInstallIsIdempotent(t *testing.T) {
	root := t.TempDir()
	s := Render(sample())

	first, err := Install(root, s)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Changed {
		t.Fatal("the first install must report a change")
	}
	second, err := Install(root, s)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed {
		t.Fatal("re-installing an identical skill must not rewrite it")
	}

	body, err := os.ReadFile(filepath.Join(root, "hive-ingestor", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != s.Body {
		t.Fatal("what landed on disk is not what was rendered")
	}
}

func TestInstallRefusesANameThatWouldEscapeTheRoot(t *testing.T) {
	// Name comes from a catalog this process did not author.
	root := t.TempDir()
	for _, bad := range []string{"../evil", "a/b", "..", ""} {
		if _, err := Install(root, Skill{Name: bad, Body: "x"}); err == nil {
			t.Fatalf("expected %q to be refused", bad)
		}
	}
}
