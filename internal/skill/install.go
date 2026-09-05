// Copyright 2026 hive-agi
// SPDX-License-Identifier: MIT

package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultRoot is where Claude Code reads user skills from.
// HIVE_SKILLS_DIR overrides it, which is what a test and a non-default
// CLAUDE_CONFIG_DIR both need.
func DefaultRoot() string {
	if d := strings.TrimSpace(os.Getenv("HIVE_SKILLS_DIR")); d != "" {
		return d
	}
	if d := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); d != "" {
		return filepath.Join(d, "skills")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "skills"
	}
	return filepath.Join(home, ".claude", "skills")
}

// Written reports what an install did to one skill.
type Written struct {
	Name    string
	Path    string
	Changed bool
}

// Install writes one skill under root. It is idempotent: a skill whose body is
// already byte-identical is left alone and reported Changed=false, so running
// it on every setup does not churn mtimes or look like work.
//
// It refuses to write outside root, because Name comes from a catalog this
// process did not author: an id carrying a path separator would otherwise
// place a file anywhere the user can write.
func Install(root string, s Skill) (Written, error) {
	w := Written{Name: s.Name, Path: s.Path(root)}
	if err := safeName(s.Name); err != nil {
		return w, err
	}
	if prior, err := os.ReadFile(w.Path); err == nil && string(prior) == s.Body {
		return w, nil
	}
	if err := os.MkdirAll(s.Dir(root), 0o755); err != nil {
		return w, fmt.Errorf("create %s: %w", s.Dir(root), err)
	}
	if err := os.WriteFile(w.Path, []byte(s.Body), 0o644); err != nil {
		return w, fmt.Errorf("write %s: %w", w.Path, err)
	}
	w.Changed = true
	return w, nil
}

// safeName rejects a skill name that could escape the skills root.
func safeName(name string) error {
	switch {
	case name == "", name == ".", name == "..":
		return fmt.Errorf("refusing to install a skill named %q", name)
	case strings.ContainsRune(name, os.PathSeparator),
		strings.ContainsRune(name, '/'),
		strings.ContainsRune(name, '\\'):
		return fmt.Errorf("refusing to install a skill whose name carries a path separator: %q", name)
	}
	return nil
}

// InstallAll writes every skill under root, reporting each.
func InstallAll(root string, skills []Skill) ([]Written, error) {
	out := make([]Written, 0, len(skills))
	for _, s := range skills {
		w, err := Install(root, s)
		if err != nil {
			return out, err
		}
		out = append(out, w)
	}
	return out, nil
}
