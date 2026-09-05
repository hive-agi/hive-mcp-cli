// Package addon scaffolds and inspects addons a user owns.
//
// Registering an addon with hive is not an API call and needs no account: the
// mounter scans the classpath for META-INF/hive-addons/*.edn and mounts what it
// finds, whoever wrote it. So "register my own addon" is two files, and this
// package writes them.
//
// The generators are pure (Manifest, Source), so what they emit is asserted in
// tests without touching a disk.
package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Spec describes the addon to scaffold.
type Spec struct {
	// ID is the addon id, e.g. "acme.rank". It doubles as the namespace.
	ID string
	// Extends is the addon id this one decorates, or "" for a standalone addon.
	Extends string
	// Capabilities the addon declares, in the ecosystem's namespaced vocabulary.
	Capabilities []string
}

// NsPath is the source path for an addon id: acme.rank -> acme/rank.clj, with
// hyphens munged to underscores as the Clojure loader expects.
func NsPath(id string) string {
	return filepath.Join(strings.Split(strings.ReplaceAll(id, "-", "_"), ".")...) + ".clj"
}

// ManifestName is the manifest file name for an addon id.
func ManifestName(id string) string {
	return strings.ReplaceAll(id, ".", "-") + ".edn"
}

func edn(strs []string) string {
	if len(strs) == 0 {
		return "#{}"
	}
	quoted := make([]string, 0, len(strs))
	for _, s := range strs {
		quoted = append(quoted, ":"+s)
	}
	return "#{" + strings.Join(quoted, " ") + "}"
}

// Manifest is the mount manifest, the file that makes this an addon.
//
// :addon/trust-class is :foss because the author owns it: a trust class of
// :proprietary makes the spec GATED, and it would then refuse to mount on the
// author's own machine until a licence gate permitted it.
func Manifest(s Spec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "{:addon/id %q\n", s.ID)
	fmt.Fprintf(&b, " :addon/type :native\n")
	fmt.Fprintf(&b, " :addon/init-ns %q\n", s.ID)
	fmt.Fprintf(&b, " :addon/init-fn \"addon-ctor\"\n")
	fmt.Fprintf(&b, " :addon/config {:addon/id %q}\n", s.ID)
	fmt.Fprintf(&b, " :addon/capabilities %s\n", edn(s.Capabilities))
	if s.Extends != "" {
		fmt.Fprintf(&b, " ;; The mounter orders %s before this addon and injects its\n", s.Extends)
		fmt.Fprintf(&b, " ;; live instance under :mount/dependencies.\n")
		fmt.Fprintf(&b, " :addon/dependencies #{%q}\n", s.Extends)
	}
	fmt.Fprintf(&b, " :addon/trust-class :foss}\n")
	return b.String()
}

// Source is the addon namespace: an IAddon implementation and the constructor
// the manifest names.
func Source(s Spec) string {
	var b strings.Builder
	fmt.Fprintf(&b, "(ns %s\n", s.ID)
	if s.Extends != "" {
		fmt.Fprintf(&b, "  \"An addon that extends %s by composing it.\n\n", s.Extends)
		fmt.Fprintf(&b, "   The mounter injects the addon this one depends on under\n")
		fmt.Fprintf(&b, "   :mount/dependencies, so the vendor's code is CALLED and never\n")
		fmt.Fprintf(&b, "   edited. Its jar stays sealed and this file is yours.\"\n")
	} else {
		fmt.Fprintf(&b, "  \"A hive addon.\"\n")
	}
	fmt.Fprintf(&b, "  (:require [hive-addon.protocol :as proto]))\n\n")

	if s.Extends != "" {
		fmt.Fprintf(&b, ";; nil when %s did not mount: a licence that does not cover it,\n", s.Extends)
		fmt.Fprintf(&b, ";; or an addon that failed to start. Your addon still mounts, so decide\n")
		fmt.Fprintf(&b, ";; here what it does without the one it wraps.\n")
	}
	fmt.Fprintf(&b, "(defrecord Addon [id inner]\n")
	fmt.Fprintf(&b, "  proto/IAddon\n")
	fmt.Fprintf(&b, "  (addon-id [_] id)\n")
	fmt.Fprintf(&b, "  (addon-type [_] :native)\n")
	fmt.Fprintf(&b, "  (capabilities [_] %s)\n", edn(s.Capabilities))
	fmt.Fprintf(&b, "  (initialize! [_ _config]\n")
	fmt.Fprintf(&b, "    {:success? true})\n")
	fmt.Fprintf(&b, "  (shutdown! [_] nil)\n")
	fmt.Fprintf(&b, "  (tools [_] [])\n")
	fmt.Fprintf(&b, "  (schema-extensions [_] [])\n")
	fmt.Fprintf(&b, "  (health [_] {:status :ok})\n")
	fmt.Fprintf(&b, "  (excluded-tools [_] #{})\n")
	fmt.Fprintf(&b, "  (hooks [_] {}))\n\n")

	fmt.Fprintf(&b, "(defn addon-ctor\n")
	fmt.Fprintf(&b, "  \"Build the addon from its mount config. Named by the manifest, and\n")
	fmt.Fprintf(&b, "   resolved by the mounter through requiring-resolve.\"\n")
	fmt.Fprintf(&b, "  [config]\n")
	if s.Extends != "" {
		fmt.Fprintf(&b, "  (->Addon (:addon/id config)\n")
		fmt.Fprintf(&b, "           (get (:mount/dependencies config) %q)))\n", s.Extends)
	} else {
		fmt.Fprintf(&b, "  (->Addon (:addon/id config) nil))\n")
	}
	return b.String()
}

// Files is what `new` writes, as path -> content, relative to the project root.
func Files(s Spec) map[string]string {
	return map[string]string{
		filepath.Join("src", NsPath(s.ID)):                                        Source(s),
		filepath.Join("resources", "META-INF", "hive-addons", ManifestName(s.ID)): Manifest(s),
	}
}

// Write puts Files(s) under root. It never overwrites: an existing file is
// reported and left alone, because the whole point of this command is to start
// a file the author then owns.
func Write(root string, s Spec) ([]string, error) {
	files := Files(s)
	paths := make([]string, 0, len(files))
	for rel := range files {
		paths = append(paths, rel)
	}
	for _, rel := range paths {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			return nil, fmt.Errorf("%s already exists: refusing to overwrite it", rel)
		}
	}
	written := make([]string, 0, len(files))
	for _, rel := range paths {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, err
		}
		if err := os.WriteFile(full, []byte(files[rel]), 0o644); err != nil {
			return nil, err
		}
		written = append(written, rel)
	}
	return written, nil
}

// Coordinate renders the deps.edn entry for a store coordinate, plus the
// private registry it is served from.
//
// Printed rather than written into the user's deps.edn: that file routinely
// carries hand-maintained aliases and comments, and a tool that rewrites it is
// a tool that eventually loses one of them.
func Coordinate(coordinate, version, storeURL string) string {
	if version == "" {
		version = "RELEASE"
	}
	return fmt.Sprintf(`Add to :deps in deps.edn

  %s {:mvn/version "%s"}

and, for a paid addon, the registry it is served from:

  :mvn/repos {"hive-store" {:url "%s/maven"}}

with your token in ~/.m2/settings.xml as the server "hive-store".
`, coordinate, version, strings.TrimRight(storeURL, "/"))
}
