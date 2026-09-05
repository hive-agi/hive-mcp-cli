// Copyright 2026 hive-agi
// SPDX-License-Identifier: MIT

// Package skill renders a Claude Code skill for an addon from the catalog the
// store already publishes.
//
// The skill is a PROJECTION, not a second document. Every sentence it emits is
// derived from a field the store sends (blurb, highlights, capabilities,
// requires, extension points), so a skill cannot claim something the catalog
// does not, and an addon whose manifest changes ships a changed skill without
// anyone editing prose. Hand-written per-addon guides were the alternative and
// they are a second copy of knowledge that drifts.
//
// What the renderer must NOT do is invent commands. It names tools only where
// the addon's own capability vocabulary names them, because a skill that
// guesses a command teaches a subscriber to paste something that does not run.
//
// The renderers are pure (Render, Frontmatter, Body), so what they emit is
// asserted in tests without touching a disk.
package skill

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hive-agi/hive-mcp-cli/internal/store"
)

// Skill is one rendered SKILL.md and where it belongs on disk.
type Skill struct {
	// Name is the skill's directory name, e.g. "hive-carto".
	Name string
	// Body is the complete SKILL.md, frontmatter included.
	Body string
}

// Dir is the skill's directory under a skills root.
func (s Skill) Dir(root string) string { return filepath.Join(root, s.Name) }

// Path is the SKILL.md path under a skills root.
func (s Skill) Path(root string) string { return filepath.Join(s.Dir(root), "SKILL.md") }

// SkillName is the skill directory name for an addon id.
//
// Ids are Maven-shaped ("io.github.hive-agi:hive-carto"); the skill takes the
// artifact half, which is the name a person actually says.
func SkillName(addonID string) string {
	name := addonID
	if i := strings.LastIndex(name, ":"); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// triggerPhrases are what a model matches this skill on. Built from the
// addon's own words rather than a hand list: its name, its tags, and the
// capabilities its manifest declares.
func triggerPhrases(a store.Addon) []string {
	seen := map[string]bool{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(a.Name)
	for _, t := range a.Tags {
		add(t)
	}
	for _, c := range a.Capabilities {
		// Capabilities keep their namespace; the readable half is what a
		// person would type.
		if i := strings.LastIndex(c, "/"); i >= 0 {
			c = c[i+1:]
		}
		add(strings.ReplaceAll(c, "-", " "))
	}
	return out
}

// Frontmatter is the YAML block Claude Code reads to decide relevance.
func Frontmatter(a store.Addon) string {
	desc := strings.TrimSpace(a.Blurb)
	if desc == "" {
		desc = fmt.Sprintf("The %s addon.", a.Name)
	}
	// Frontmatter is YAML and the blurb is prose that may carry a colon, which
	// would end the key. Quote it and escape the quotes rather than hoping.
	desc = strings.ReplaceAll(desc, `"`, `\"`)
	desc = strings.Join(strings.Fields(desc), " ")

	triggers := triggerPhrases(a)
	if len(triggers) > 0 {
		desc += " Triggers on: " + strings.Join(triggers, ", ") + "."
	}

	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", SkillName(a.ID))
	fmt.Fprintf(&b, "description: \"%s\"\n", desc)
	b.WriteString("---\n")
	return b.String()
}

func bullets(b *strings.Builder, items []string) {
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
}

// statusLine says plainly whether the reader can resolve this today. The
// store's status means "a customer's token resolves this coordinate", so
// preview is a fact about the registry and not a quality judgement.
func statusLine(a store.Addon) string {
	switch a.Status {
	case "available":
		return fmt.Sprintf("`%s` resolves for an entitled token today.", a.Coordinate)
	case "preview":
		return fmt.Sprintf("`%s` is PREVIEW: it is described here but does not resolve yet, "+
			"so a dependency on it will not fetch.", a.Coordinate)
	default:
		return fmt.Sprintf("`%s` has status %q.", a.Coordinate, a.Status)
	}
}

// Body renders the skill's markdown, frontmatter included.
func Body(a store.Addon) string {
	var b strings.Builder
	b.WriteString(Frontmatter(a))
	fmt.Fprintf(&b, "\n# %s\n\n%s\n\n", a.Name, strings.TrimSpace(a.Blurb))

	fmt.Fprintf(&b, "## Whether you can use it\n\n%s\n", statusLine(a))
	if a.Version != "" {
		fmt.Fprintf(&b, "\nThe registry last answered version `%s`.\n", a.Version)
	}
	fmt.Fprintf(&b, "\nCoordinate for `deps.edn`:\n\n```clojure\n%s {:mvn/version \"%s\"}\n```\n",
		a.Coordinate, versionOr(a.Version, "RELEASE"))

	if len(a.Highlights) > 0 {
		b.WriteString("\n## What it gives you\n\n")
		bullets(&b, a.Highlights)
	}

	if len(a.Capabilities) > 0 {
		caps := append([]string(nil), a.Capabilities...)
		sort.Strings(caps)
		b.WriteString("\n## Capabilities it declares\n\n")
		b.WriteString("These are the ecosystem's vocabulary, declared in this addon's own\n")
		b.WriteString("manifest. Another addon asking for one of them is satisfied by this one.\n\n")
		bullets(&b, backticked(caps))
	}

	if len(a.RequiresCapabilities) > 0 {
		reqs := append([]string(nil), a.RequiresCapabilities...)
		sort.Strings(reqs)
		b.WriteString("\n## Capabilities it needs\n\n")
		b.WriteString("Something on the classpath must provide each of these or the mount\n")
		b.WriteString("reports the seam as unsatisfied.\n\n")
		bullets(&b, backticked(reqs))
	}

	if len(a.Requires) > 0 {
		b.WriteString("\n## Addons it needs\n\n")
		bullets(&b, backticked(a.Requires))
	}
	if len(a.RequiredBy) > 0 {
		b.WriteString("\n## Addons built on it\n\n")
		b.WriteString("Each of these resolves through this one, so entitlement to a descendant\n")
		b.WriteString("means nothing without this addon underneath it.\n\n")
		bullets(&b, backticked(a.RequiredBy))
	}

	writeExtensionPoints(&b, a)

	b.WriteString("\n## Where this text comes from\n\n")
	b.WriteString("Rendered by `hive addon skill` from the storefront catalog, so it says what\n")
	b.WriteString("the addon's own manifest says and nothing more. It deliberately does NOT\n")
	b.WriteString("invent tool invocations: ask the running host (`help` on the tool) for the\n")
	b.WriteString("command surface, because that listing is derived from the live handler\n")
	b.WriteString("table and cannot go stale, and a pasted command that does not run is worse\n")
	b.WriteString("than no example.\n")
	if a.Docs != "" {
		fmt.Fprintf(&b, "\nUpstream docs: %s\n", a.Docs)
	}
	return b.String()
}

func writeExtensionPoints(b *strings.Builder, a store.Addon) {
	if len(a.ExtensionPoints) == 0 {
		return
	}
	b.WriteString("\n## Extending it\n\n")
	b.WriteString("This addon opens the seams below. You fill one by writing your OWN addon\n")
	b.WriteString("that implements the port and registers itself, which needs no account and\n")
	b.WriteString("no permission: the mounter scans the classpath for\n")
	b.WriteString("`META-INF/hive-addons/*.edn` and mounts what it finds, whoever wrote it.\n")
	b.WriteString("`hive addon new --extends " + a.ID + "` writes both files.\n")

	pts := append([]store.ExtensionPoint(nil), a.ExtensionPoints...)
	sort.Slice(pts, func(i, j int) bool { return pts[i].Capability < pts[j].Capability })
	for _, p := range pts {
		fmt.Fprintf(b, "\n### `%s`\n\n%s\n\n", p.Capability, strings.TrimSpace(p.Summary))
		if p.Port != "" {
			fmt.Fprintf(b, "- Port to implement: `%s`\n", p.Port)
		}
		if p.Registry != "" {
			fmt.Fprintf(b, "- Register with: `%s`\n", p.Registry)
		}
		if p.Cardinality != "" {
			switch p.Cardinality {
			case "one":
				b.WriteString("- Cardinality: ONE. A second provider replaces the first rather than joining it.\n")
			case "many":
				b.WriteString("- Cardinality: many. Providers accumulate.\n")
			default:
				fmt.Fprintf(b, "- Cardinality: %s\n", p.Cardinality)
			}
		}
		if len(p.FilledBy) == 0 {
			b.WriteString("- Filled by: NOTHING yet. The seam is declared and vacant, so yours\n")
			b.WriteString("  would be the first, and nothing exercises it today.\n")
		} else {
			fmt.Fprintf(b, "- Filled by: %s\n", strings.Join(backticked(p.FilledBy), ", "))
		}
	}
}

func backticked(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, "`"+x+"`")
	}
	return out
}

func versionOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

// Render builds the skill for one addon.
func Render(a store.Addon) Skill {
	return Skill{Name: SkillName(a.ID), Body: Body(a)}
}

// RenderAll builds a skill per addon, in catalog order.
func RenderAll(addons []store.Addon) []Skill {
	out := make([]Skill, 0, len(addons))
	for _, a := range addons {
		out = append(out, Render(a))
	}
	return out
}
