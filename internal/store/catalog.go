// Package store is a read client for the hive-store storefront.
//
// The shapes here mirror hive-store.boundary.contract, which is the schema the
// store and its web UI already agree on. Field names are the wire names, so a
// key renamed on the store side fails to decode rather than reading as an empty
// string.
//
// This package decides nothing about entitlement. It fetches and it displays.
// Whether an addon may MOUNT is decided by hive-license's gate inside the JVM
// that mounts it, and a second answer computed here in Go would be a second
// licence decision in the fleet, which is the outcome hive-license exists to
// prevent.
package store

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// DefaultBaseURL is the public storefront. HIVE_STORE_URL overrides it, which
// is what a self-hosted store or a local dev server needs.
//
// The apex is NOT it. hive-mcp.com serves the marketing site and install.sh
// and nothing else: /healthz and every /api path answer 404. Pointing here
// made every catalog command fail for anyone who had not set the environment
// variable, which is everyone on their first run.
const DefaultBaseURL = "https://store.hive-mcp.com"

// BaseURL is the store this CLI talks to.
func BaseURL() string {
	if u := strings.TrimSpace(os.Getenv("HIVE_STORE_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	return DefaultBaseURL
}

// Token is the customer's store token, used as a bearer credential. Absent for
// anonymous browsing, which the catalog allows.
func Token() string {
	return strings.TrimSpace(os.Getenv("HIVE_STORE_TOKEN"))
}

// ExtensionPoint is one seam an addon opens for providers.
//
// FilledBy is EMPTY rather than absent when nothing fills the seam: absent
// would read as a point carrying no such fact, and empty is the fact a reader
// asking whether they could write a provider actually needs.
type ExtensionPoint struct {
	Capability  string   `json:"capability"`
	Summary     string   `json:"summary"`
	Port        string   `json:"port"`
	Registry    string   `json:"registry"`
	Cardinality string   `json:"cardinality"`
	FilledBy    []string `json:"filledBy"`
}

// OpenAddon is one openly published addon the store points at and does not
// sell. It has no price, status, version or coordinate, because none of those
// are facts about something this store neither gates nor ships.
type OpenAddon struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Blurb        string   `json:"blurb"`
	Repo         string   `json:"repo"`
	License      string   `json:"license"`
	OpenSource   bool     `json:"openSource"`
	Tags         []string `json:"tags"`
	Capabilities []string `json:"capabilities"`
	Requires     []string `json:"requires"`
	RequiredBy   []string `json:"requiredBy"`
}

// Addon is one entry on the storefront.
//
// Capabilities keep their namespace ("universe/engine", not "engine") because
// they are the ecosystem's vocabulary, declared by each project's manifest.
type Addon struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"name"`
	Blurb                string   `json:"blurb"`
	Coordinate           string   `json:"coordinate"`
	Docs                 string   `json:"docs"`
	Status               string   `json:"status"`
	Kind                 string   `json:"kind"`
	Tags                 []string `json:"tags"`
	Highlights           []string `json:"highlights"`
	Version              string   `json:"version"`
	Capabilities         []string `json:"capabilities"`
	RequiresCapabilities []string `json:"requiresCapabilities"`
	Requires             []string `json:"requires"`
	RequiredBy           []string `json:"requiredBy"`
	// Seams this addon opens for providers. The store sends these; this client
	// did not decode them until 2026-09-05, which is the drift a wire contract
	// catches in one direction only: a RENAMED key fails to decode loudly, an
	// ADDED key is silently dropped.
	ExtensionPoints []ExtensionPoint `json:"extensionPoints"`
}

// Catalog is the storefront as one response. Only the parts a CLI reads are
// decoded; the store sends plans, usages and payment rails too.
type Catalog struct {
	Addons  []Addon  `json:"addons"`
	Shelves []string `json:"shelves"`
	// Openly published addons the store points at and does not sell. A separate
	// list so nothing walking Addons to build a shelf or a price reaches one.
	OpenAddons []OpenAddon `json:"openAddons"`
}

// Me is who the store thinks is asking, and what they are entitled to.
type Me struct {
	Email        string        `json:"email"`
	Subscription *Subscription `json:"subscription"`
}

// Subscription carries the entitlement as the store resolves it AT THE MOMENT
// OF ASKING: a lapsed period reads "expired" and entitles nothing, even though
// no job demoted the row.
type Subscription struct {
	Plan     string   `json:"plan"`
	Status   string   `json:"status"`
	Provider string   `json:"provider"`
	Entitles []string `json:"entitles"`
}

// Client reads the store over HTTP.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New builds a client from the environment.
func New() *Client {
	return &Client{
		BaseURL: BaseURL(),
		Token:   Token(),
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *Client) get(ctx context.Context, path string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", c.BaseURL, path, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%s: not authorized. Set HIVE_STORE_TOKEN to a store token", path)
	case resp.StatusCode >= 400:
		// Name the host, not just the path: the commonest cause of a 404 here
		// is a HIVE_STORE_URL aimed at something that is not the store, and an
		// error that hides which URL it tried cannot tell you that.
		return fmt.Errorf("%s%s: store answered %s (set HIVE_STORE_URL to point elsewhere)",
			c.BaseURL, path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		return fmt.Errorf("%s: could not read the store's answer: %w", path, err)
	}
	return nil
}

// Catalog fetches the storefront. Anonymous requests are allowed, and see the
// public plans only.
func (c *Client) Catalog(ctx context.Context) (*Catalog, error) {
	var cat Catalog
	if err := c.get(ctx, "/api/catalog", &cat); err != nil {
		return nil, err
	}
	return &cat, nil
}

// Me fetches the caller's identity and entitlement. Requires a token.
func (c *Client) Me(ctx context.Context) (*Me, error) {
	if c.Token == "" {
		return nil, fmt.Errorf("no store token: set HIVE_STORE_TOKEN")
	}
	var me Me
	if err := c.get(ctx, "/api/me", &me); err != nil {
		return nil, err
	}
	return &me, nil
}

// ── Search: pure, so it is testable without a store ───────────────────────

// Match reports whether addon `a` matches the free-text `query`, which is
// matched against the id, name, blurb, tags and capabilities. An empty query
// matches everything, so `hive addon search` with no argument lists the shelf.
func Match(a Addon, query string) bool {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	for _, field := range a.searchable() {
		if strings.Contains(strings.ToLower(field), q) {
			return true
		}
	}
	return false
}

func (a Addon) searchable() []string {
	fields := []string{a.ID, a.Name, a.Blurb, a.Coordinate}
	fields = append(fields, a.Tags...)
	fields = append(fields, a.Capabilities...)
	return fields
}

// Search returns the addons matching `query`, and those on `shelf` when it is
// non-empty, in a stable order: available before preview, then by id.
//
// Ordering is decided here rather than trusted from the response, because two
// runs of the same search printing different orders is the thing that makes a
// CLI feel broken.
func Search(addons []Addon, query, shelf string) []Addon {
	out := make([]Addon, 0, len(addons))
	for _, a := range addons {
		if !Match(a, query) {
			continue
		}
		if shelf != "" && !hasTag(a, shelf) {
			continue
		}
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool {
		li, lj := rank(out[i]), rank(out[j])
		if li != lj {
			return li < lj
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func hasTag(a Addon, shelf string) bool {
	for _, t := range a.Tags {
		if strings.EqualFold(t, shelf) {
			return true
		}
	}
	return false
}

func rank(a Addon) int {
	if a.Status == "available" {
		return 0
	}
	return 1
}

// Find returns the addon with `id`, or false. The id is matched exactly and
// then, failing that, as a suffix, so "carto" finds "hive.carto" without
// silently picking one of several partial hits.
func Find(addons []Addon, id string) (Addon, bool) {
	for _, a := range addons {
		if a.ID == id {
			return a, true
		}
	}
	// An exact match on the ARTIFACT name wins before anything fuzzy. Without
	// this pass "hive-ingestor" is ambiguous against "hive-ingestor-rfc",
	// because the fuzzy pass below is a substring test and every addon whose
	// name a shorthand PREFIXES matches it too. Any addon with a descendant
	// named after it was unreachable by its own name.
	for _, a := range addons {
		if artifact(a.ID) == id {
			return a, true
		}
	}

	var hit Addon
	found := 0
	for _, a := range addons {
		if strings.HasSuffix(a.ID, "."+id) || strings.Contains(a.Coordinate, id) {
			hit = a
			found++
		}
	}
	if found == 1 {
		return hit, true
	}
	return Addon{}, false
}

// artifact is the half of a Maven-shaped id a person actually says:
// "io.github.hive-agi:hive-carto" -> "hive-carto".
func artifact(id string) string {
	if i := strings.LastIndexAny(id, ":/"); i >= 0 {
		return id[i+1:]
	}
	return id
}

// Entitled reports whether `sub` entitles `id` right now. A nil or non-active
// subscription entitles nothing.
func Entitled(sub *Subscription, id string) bool {
	if sub == nil || sub.Status != "active" {
		return false
	}
	for _, e := range sub.Entitles {
		if e == id {
			return true
		}
	}
	return false
}
