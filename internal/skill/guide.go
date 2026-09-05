// Copyright 2026 hive-agi
// SPDX-License-Identifier: MIT

// Guides are the setup skills compiled into this binary.
//
// They are not catalog projections like the per-addon skills next door: nothing
// in a store response describes how to install a JVM host, so these are prose,
// and they are embedded rather than fetched so `hive guide --install` works on a
// machine that has no network, no token and no hive-mcp yet. That is the whole
// point of them: they are what a first run leaves behind for Claude to read.
package skill

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed guides/*/SKILL.md
var guideFS embed.FS

// Guides returns every embedded guide, name-sorted.
func Guides() ([]Skill, error) {
	entries, err := fs.ReadDir(guideFS, "guides")
	if err != nil {
		return nil, fmt.Errorf("read embedded guides: %w", err)
	}
	out := make([]Skill, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := readGuide(e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Guide returns the one guide named name.
func Guide(name string) (Skill, error) {
	guides, err := Guides()
	if err != nil {
		return Skill{}, err
	}
	for _, g := range guides {
		if g.Name == name {
			return g, nil
		}
	}
	names := make([]string, 0, len(guides))
	for _, g := range guides {
		names = append(names, g.Name)
	}
	return Skill{}, fmt.Errorf("no guide named %q; have %s", name, strings.Join(names, ", "))
}

// GuideSummary is the one-line description a guide's frontmatter carries.
func GuideSummary(s Skill) string {
	desc := frontmatterField(s.Body, "description")
	if i := strings.IndexRune(desc, '.'); i > 0 {
		return desc[:i+1]
	}
	return desc
}

func readGuide(name string) (Skill, error) {
	body, err := guideFS.ReadFile(path.Join("guides", name, "SKILL.md"))
	if err != nil {
		return Skill{}, fmt.Errorf("read guide %s: %w", name, err)
	}
	return Skill{Name: name, Body: string(body)}, nil
}

// frontmatterField reads one scalar key out of the leading YAML block. It
// handles exactly the shape these files are written in (`key: value` on one
// line), and returns "" for anything else rather than pulling in a parser.
func frontmatterField(body, key string) string {
	if !strings.HasPrefix(body, "---\n") {
		return ""
	}
	end := strings.Index(body[4:], "\n---")
	if end < 0 {
		return ""
	}
	for _, line := range strings.Split(body[4:4+end], "\n") {
		if rest, ok := strings.CutPrefix(line, key+":"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}
