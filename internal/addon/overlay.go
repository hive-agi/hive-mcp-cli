package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// The host's personal overlay is ~/hive-mcp/local.deps.edn: gitignored, and
// merged over starter.deps.edn by bin/hive-mcp-foss at every boot. It is where
// a bought addon's coordinate and the hive-store repo go.
//
// Same rule as Coordinate: a file the user already wrote is never rewritten,
// because it may carry comments and pins a rewrite would drop. This creates the
// file when there is none, recognises an entry that is already there, and
// otherwise hands back the lines to paste.

// OverlayResult says what AddToOverlay did.
type OverlayResult int

const (
	OverlayCreated OverlayResult = iota // there was no overlay; it now holds this addon
	OverlayPresent                      // the coordinate is already in the overlay
	OverlayManual                       // an overlay exists and was left alone; paste Snippet
)

// OverlayPath is the overlay for a hive-mcp checkout.
func OverlayPath(hiveMCPDir string) string {
	return filepath.Join(hiveMCPDir, "local.deps.edn")
}

// AddToOverlay puts coordinate at version, and the store repo, in the overlay
// at path. The snippet is returned in every case, so a caller can always show it.
func AddToOverlay(path, coordinate, version, storeURL string) (OverlayResult, string, error) {
	if version == "" {
		version = "RELEASE"
	}
	repo := fmt.Sprintf(`"hive-store" {:url "%s/maven"}`, strings.TrimRight(storeURL, "/"))
	dep := fmt.Sprintf(`%s {:mvn/version "%s"}`, coordinate, version)
	snippet := fmt.Sprintf("  :mvn/repos {%s}\n  :deps {%s}\n", repo, dep)

	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Starts with "{": the clojure CLI reads an -Sdeps argument that does
		// not as a file PATH, so the comments go inside the map.
		body := fmt.Sprintf("{;; Personal overlay, merged over starter.deps.edn by bin/hive-mcp-foss.\n"+
			" ;; Gitignored. Your store token is NOT here: it lives in ~/.m2/settings.xml.\n"+
			" :mvn/repos {%s}\n :deps {%s}}\n", repo, dep)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return 0, snippet, err
		}
		return OverlayCreated, snippet, nil
	}
	if err != nil {
		return 0, snippet, err
	}
	// The coordinate as a whole symbol, not a prefix of a longer one
	// (hive-carto must not match hive-carto-flow).
	re := regexp.MustCompile(`(^|[\s{,])` + regexp.QuoteMeta(coordinate) + `($|[\s{,])`)
	if re.Match(existing) {
		return OverlayPresent, snippet, nil
	}
	return OverlayManual, snippet, nil
}
