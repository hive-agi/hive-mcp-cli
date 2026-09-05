// Copyright 2026 hive-agi
// SPDX-License-Identifier: MIT

package skill

import (
	"strings"
	"testing"
)

// Every guide must carry the frontmatter Claude Code matches a skill on. A
// guide whose `name` disagrees with its directory is installed under one name
// and announces another, which is the failure this pins.
func TestGuidesCarryMatchingFrontmatter(t *testing.T) {
	guides, err := Guides()
	if err != nil {
		t.Fatalf("Guides: %v", err)
	}
	if len(guides) == 0 {
		t.Fatal("no guides embedded; the go:embed pattern matched nothing")
	}
	for _, g := range guides {
		if !strings.HasPrefix(g.Body, "---\n") {
			t.Errorf("%s: body does not open with YAML frontmatter", g.Name)
		}
		if got := frontmatterField(g.Body, "name"); got != g.Name {
			t.Errorf("%s: frontmatter name is %q, want the directory name", g.Name, got)
		}
		if desc := frontmatterField(g.Body, "description"); len(desc) < 40 {
			t.Errorf("%s: description is %q, too short to match on", g.Name, desc)
		}
		if err := safeName(g.Name); err != nil {
			t.Errorf("%s: %v", g.Name, err)
		}
	}
}

// The two guides the installer promises must exist under exactly these names:
// install.sh and the README both tell people to say a sentence that only
// resolves if the skill is there.
func TestGuidesIncludeTheAdvertisedOnes(t *testing.T) {
	for _, name := range []string{"hive-mcp-setup", "hive-store"} {
		if _, err := Guide(name); err != nil {
			t.Errorf("Guide(%q): %v", name, err)
		}
	}
}

func TestGuideRejectsAnUnknownName(t *testing.T) {
	_, err := Guide("no-such-guide")
	if err == nil {
		t.Fatal("want an error naming what is available")
	}
	if !strings.Contains(err.Error(), "hive-mcp-setup") {
		t.Errorf("error should list the available guides, got %q", err)
	}
}

func TestGuideSummaryIsOneSentence(t *testing.T) {
	g, err := Guide("hive-store")
	if err != nil {
		t.Fatal(err)
	}
	got := GuideSummary(g)
	if got == "" || strings.Count(got, ".") != 1 {
		t.Errorf("GuideSummary = %q, want one sentence", got)
	}
}

// A guide installs like any other skill, and a second install of unchanged
// content is a no-op: the installer runs on every setup.
func TestGuidesInstallIdempotently(t *testing.T) {
	root := t.TempDir()
	guides, err := Guides()
	if err != nil {
		t.Fatal(err)
	}
	first, err := InstallAll(root, guides)
	if err != nil {
		t.Fatalf("first install: %v", err)
	}
	for _, w := range first {
		if !w.Changed {
			t.Errorf("%s: first install reported unchanged", w.Name)
		}
	}
	second, err := InstallAll(root, guides)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	for _, w := range second {
		if w.Changed {
			t.Errorf("%s: second install rewrote an identical file", w.Name)
		}
	}
}
