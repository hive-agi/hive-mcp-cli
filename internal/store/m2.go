package store

// The artifact token's home: ~/.m2/settings.xml.
//
// A subscriber used to be handed a settings.xml block to paste by hand, with
// three warnings about how it goes wrong: the <id> must equal the deps.edn repo
// key, the token goes in BOTH <username> and <password>, and a global
// ~/.clojure/deps.edn defining the same repo shadows the project's. Every one of
// those is mechanical, so this file does it and `hive store login --check`
// reports it.

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// RepoID is the Maven server id and the deps.edn :mvn/repos key. It must be the
// same string in both places or Maven never attaches the credential; the store
// renders its own setup page with this id (hive-store-ui.manifest/repo-id).
const RepoID = "hive-store"

// MavenURL is the gateway that checks the artifact token.
func MavenURL() string { return BaseURL() + "/maven" }

// SettingsPath is where Maven and tools.deps read server credentials.
func SettingsPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".m2", "settings.xml")
}

var (
	serverIDRe = regexp.MustCompile(`(?s)<id>\s*([^<]*?)\s*</id>`)
	usernameRe = regexp.MustCompile(`(?s)<username>\s*([^<]*?)\s*</username>`)
	passwordRe = regexp.MustCompile(`(?s)<password>\s*([^<]*?)\s*</password>`)
)

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func serverBlock(indent, id, token string) string {
	t := xmlEscape(token)
	return fmt.Sprintf("%[1]s<server>\n%[1]s  <id>%[2]s</id>\n%[1]s  <username>%[3]s</username>\n%[1]s  <password>%[3]s</password>\n%[1]s</server>\n",
		indent, xmlEscape(id), t)
}

// findServer returns the byte range of the <server> block whose <id> is id.
func findServer(doc, id string) (int, int, bool) {
	pos := 0
	for {
		start := strings.Index(doc[pos:], "<server>")
		if start < 0 {
			return 0, 0, false
		}
		start += pos
		end := strings.Index(doc[start:], "</server>")
		if end < 0 {
			return 0, 0, false
		}
		end += start + len("</server>")
		if m := serverIDRe.FindStringSubmatch(doc[start:end]); m != nil && m[1] == id {
			return start, end, true
		}
		pos = end
	}
}

// UpsertServer returns doc with exactly one <server> for id, carrying token as
// both username and password. Every other byte of an existing file is kept:
// other servers, mirrors, profiles and comments are the user's, not ours.
func UpsertServer(doc, id, token string) (string, error) {
	if strings.TrimSpace(doc) == "" {
		return "<settings>\n  <servers>\n" + serverBlock("    ", id, token) + "  </servers>\n</settings>\n", nil
	}
	if start, end, ok := findServer(doc, id); ok {
		// Keep the block's own indentation so the file still reads as written.
		lineStart := strings.LastIndex(doc[:start], "\n") + 1
		indent := doc[lineStart:start]
		if strings.TrimSpace(indent) != "" {
			indent, lineStart = "    ", start
		}
		after := end
		if after < len(doc) && doc[after] == '\n' {
			after++
		}
		return doc[:lineStart] + serverBlock(indent, id, token) + doc[after:], nil
	}
	if i := strings.Index(doc, "</servers>"); i >= 0 {
		lineStart := strings.LastIndex(doc[:i], "\n") + 1
		return doc[:lineStart] + serverBlock("    ", id, token) + doc[lineStart:], nil
	}
	if i := strings.LastIndex(doc, "</settings>"); i >= 0 {
		lineStart := strings.LastIndex(doc[:i], "\n") + 1
		return doc[:lineStart] + "  <servers>\n" + serverBlock("    ", id, token) + "  </servers>\n" + doc[lineStart:], nil
	}
	return "", errors.New("settings.xml has no </settings> element; not editing a file this tool cannot read")
}

// WriteSettings upserts the server block into the settings file at path,
// keeping a timestamped copy of any file it changes. The file holds a secret,
// so it is written 0600.
func WriteSettings(path, id, token string) (backup string, err error) {
	old, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	updated, err := UpsertServer(string(old), id, token)
	if err != nil {
		return "", err
	}
	if string(old) == updated {
		return "", nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if len(old) > 0 {
		backup = path + ".bak-" + time.Now().Format("20060102-150405")
		if err := os.WriteFile(backup, old, 0o600); err != nil {
			return "", err
		}
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o600); err != nil {
		return "", err
	}
	return backup, os.Rename(tmp, path)
}

// RemoveServerIfToken deletes the id server block from the settings file at
// path, but only when it carries token: a block the user pasted by hand, or a
// token from another login, is theirs and stays. Reports whether it removed one.
func RemoveServerIfToken(path, id, token string) (bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	doc := string(b)
	start, end, ok := findServer(doc, id)
	if !ok {
		return false, nil
	}
	if u := usernameRe.FindStringSubmatch(doc[start:end]); u == nil || u[1] != xmlEscape(token) {
		return false, nil
	}
	lineStart := strings.LastIndex(doc[:start], "\n") + 1
	if strings.TrimSpace(doc[lineStart:start]) != "" {
		lineStart = start
	}
	if end < len(doc) && doc[end] == '\n' {
		end++
	}
	updated := doc[:lineStart] + doc[end:]
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(updated), 0o600); err != nil {
		return false, err
	}
	return true, os.Rename(tmp, path)
}

// Wiring is what `hive store login --check` reports. Each field is a fact about
// this machine, not a verdict, so the caller decides how loud to be.
type Wiring struct {
	SettingsPath   string
	SettingsExists bool
	ServerFound    bool
	UserEqualsPass bool
	Token          string // the configured token, for the live probe; never printed whole
	// Shadow is set when ~/.clojure/deps.edn names the repo too. A user-level
	// definition wins silently over the project's.
	Shadow string
}

// Inspect reads the current wiring without changing anything.
func Inspect() Wiring {
	w := Wiring{SettingsPath: SettingsPath()}
	if b, err := os.ReadFile(w.SettingsPath); err == nil {
		w.SettingsExists = true
		doc := string(b)
		if start, end, ok := findServer(doc, RepoID); ok {
			w.ServerFound = true
			block := doc[start:end]
			u, p := usernameRe.FindStringSubmatch(block), passwordRe.FindStringSubmatch(block)
			if u != nil && p != nil {
				w.UserEqualsPass = u[1] == p[1]
				w.Token = u[1]
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		userDeps := filepath.Join(home, ".clojure", "deps.edn")
		if b, err := os.ReadFile(userDeps); err == nil && strings.Contains(string(b), `"`+RepoID+`"`) {
			w.Shadow = userDeps
		}
	}
	return w
}

// ProbeResult is what the gateway said about a token.
type ProbeResult int

const (
	ProbeAccepted     ProbeResult = iota // the gateway served the probe artifact
	ProbeNotEntitled                     // authenticated, but the probe artifact is not in the plan
	ProbeRejected                        // the gateway does not know this token
	ProbeInconclusive                    // anything else: network, 5xx, a probe that moved
)

// probePath is an artifact every plan's catalog lists. Its metadata is small,
// and reading it proves the credential without downloading a jar.
const probePath = "/io/github/hive-agi/hive-carto/maven-metadata.xml"

// ProbeToken asks the gateway whether it accepts token, the way Maven will:
// Basic auth, token in both positions.
func ProbeToken(ctx context.Context, token string) (ProbeResult, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, MavenURL()+probePath, nil)
	if err != nil {
		return ProbeInconclusive, err.Error()
	}
	req.SetBasicAuth(token, token)
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return ProbeInconclusive, err.Error()
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK:
		return ProbeAccepted, resp.Status
	case resp.StatusCode == http.StatusUnauthorized:
		return ProbeRejected, resp.Status
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound:
		return ProbeNotEntitled, resp.Status
	default:
		return ProbeInconclusive, resp.Status
	}
}

// Redact keeps a token recognisable in output without printing it.
func Redact(token string) string {
	if len(token) <= 12 {
		return "****"
	}
	return token[:8] + "…" + token[len(token)-4:]
}
