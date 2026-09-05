package store

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

var shelf = []Addon{
	{ID: "hive.carto", Name: "Carto", Blurb: "Code cartography.", Coordinate: "io.github.hive-agi/hive-carto",
		Status: "available", Kind: "addon", Tags: []string{"code"}, Capabilities: []string{"carto/index"}},
	{ID: "hive.knowledge", Name: "Knowledge", Blurb: "Memory and the graph.", Coordinate: "io.github.hive-agi/hive-knowledge",
		Status: "available", Kind: "addon", Tags: []string{"knowledge"}},
	{ID: "hive.assay", Name: "Assay", Blurb: "Verification for code.", Coordinate: "io.github.hive-agi/hive-assay",
		Status: "preview", Kind: "addon", Tags: []string{"verification", "code"}},
}

func ids(as []Addon) []string {
	out := make([]string, 0, len(as))
	for _, a := range as {
		out = append(out, a.ID)
	}
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestSearchWithNoQueryListsTheWholeShelf(t *testing.T) {
	// available before preview, then by id: the same search twice must print
	// the same order.
	eq(t, ids(Search(shelf, "", "")), []string{"hive.carto", "hive.knowledge", "hive.assay"})
}

func TestSearchMatchesBlurbAndCapabilityNotJustName(t *testing.T) {
	eq(t, ids(Search(shelf, "cartography", "")), []string{"hive.carto"})
	eq(t, ids(Search(shelf, "carto/index", "")), []string{"hive.carto"})
	eq(t, ids(Search(shelf, "hive-agi", "")), []string{"hive.carto", "hive.knowledge", "hive.assay"})
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	eq(t, ids(Search(shelf, "KNOWLEDGE", "")), []string{"hive.knowledge"})
}

func TestShelfNarrowsToATag(t *testing.T) {
	eq(t, ids(Search(shelf, "", "code")), []string{"hive.carto", "hive.assay"})
	eq(t, ids(Search(shelf, "", "nosuchshelf")), []string{})
}

func TestFindPrefersAnExactIDOverASuffix(t *testing.T) {
	both := append([]Addon{{ID: "carto", Name: "Ambiguous"}}, shelf...)
	a, ok := Find(both, "carto")
	if !ok || a.Name != "Ambiguous" {
		t.Fatalf("exact id must win, got %+v ok=%v", a, ok)
	}
}

func TestFindRefusesAnAmbiguousShorthand(t *testing.T) {
	// Two addons whose ids both end in ".shape": naming one of them would be a
	// coin flip the user cannot see.
	ambiguous := []Addon{{ID: "hive.shape"}, {ID: "acme.shape"}}
	if _, ok := Find(ambiguous, "shape"); ok {
		t.Fatal("an ambiguous shorthand must not resolve")
	}
}

func TestEntitledIsFalseForEveryNonActiveSubscription(t *testing.T) {
	if Entitled(nil, "hive.carto") {
		t.Fatal("no subscription entitles nothing")
	}
	lapsed := &Subscription{Status: "expired", Entitles: []string{"hive.carto"}}
	if Entitled(lapsed, "hive.carto") {
		t.Fatal("an expired subscription still listing an addon must not entitle it")
	}
	live := &Subscription{Status: "active", Entitles: []string{"hive.carto"}}
	if !Entitled(live, "hive.carto") {
		t.Fatal("an active subscription must entitle what it lists")
	}
	if Entitled(live, "hive.knowledge") {
		t.Fatal("entitlement is per addon, not per subscription")
	}
}

// ── The wire, against a server that answers as the contract says ──────────

func TestCatalogDecodesTheContractFieldNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/catalog" {
			t.Errorf("asked for %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Exactly the keys hive-store.boundary.contract/Addon declares.
		_, _ = w.Write([]byte(`{"addons":[{"id":"hive.carto","name":"Carto","blurb":"Code cartography.",
          "coordinate":"io.github.hive-agi/hive-carto","docs":"https://x/docs","status":"available",
          "kind":"addon","tags":["code"],"highlights":["fast"],"version":"0.1.396",
          "capabilities":["carto/index"],"requiresCapabilities":["vessel"],
          "requires":["hive.knowledge"],"requiredBy":[]}],"shelves":["code","knowledge"]}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	cat, err := c.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cat.Addons) != 1 {
		t.Fatalf("got %d addons", len(cat.Addons))
	}
	a := cat.Addons[0]
	// Every field asserted: a key renamed on the store side decodes to a zero
	// value in Go and would otherwise print as an empty column.
	for _, tc := range []struct{ name, got, want string }{
		{"id", a.ID, "hive.carto"},
		{"name", a.Name, "Carto"},
		{"blurb", a.Blurb, "Code cartography."},
		{"coordinate", a.Coordinate, "io.github.hive-agi/hive-carto"},
		{"docs", a.Docs, "https://x/docs"},
		{"status", a.Status, "available"},
		{"kind", a.Kind, "addon"},
		{"version", a.Version, "0.1.396"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
	eq(t, a.Tags, []string{"code"})
	eq(t, a.Capabilities, []string{"carto/index"})
	eq(t, a.RequiresCapabilities, []string{"vessel"})
	eq(t, a.Requires, []string{"hive.knowledge"})
	eq(t, cat.Shelves, []string{"code", "knowledge"})
}

func TestTheTokenTravelsAsABearerCredential(t *testing.T) {
	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"email":"a@b.c","subscription":{"plan":"pro","status":"active","provider":"stripe","entitles":["hive.carto"]}}`))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "tok-123", HTTP: srv.Client()}
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if seen != "Bearer tok-123" {
		t.Fatalf("Authorization was %q", seen)
	}
	if !Entitled(me.Subscription, "hive.carto") {
		t.Fatal("the decoded subscription must entitle what it lists")
	}
}

func TestMeWithoutATokenFailsBeforeTheRequest(t *testing.T) {
	asked := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = true
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.Me(context.Background()); err == nil {
		t.Fatal("expected an error naming the missing token")
	}
	if asked {
		t.Fatal("an anonymous /api/me must not be sent at all")
	}
}

func TestAnUnauthorizedAnswerSaysWhatToSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, Token: "stale", HTTP: srv.Client()}
	_, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if want := "HIVE_STORE_TOKEN"; !contains(err.Error(), want) {
		t.Fatalf("error %q should name %s", err, want)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestFindPrefersAnExactArtifactNameOverAFuzzyMatch(t *testing.T) {
	// "hive-ingestor" is a PREFIX of "hive-ingestor-rfc", so the substring
	// pass matches both and reports ambiguity. Any addon with a descendant
	// named after it was unreachable by its own name.
	addons := []Addon{
		{ID: "io.github.hive-agi:hive-ingestor", Coordinate: "io.github.hive-agi/hive-ingestor"},
		{ID: "io.github.hive-agi:hive-ingestor-rfc", Coordinate: "io.github.hive-agi/hive-ingestor-rfc"},
	}
	got, ok := Find(addons, "hive-ingestor")
	if !ok {
		t.Fatal("hive-ingestor must resolve to itself, not read as ambiguous")
	}
	if got.ID != "io.github.hive-agi:hive-ingestor" {
		t.Fatalf("resolved to %q", got.ID)
	}
	// the descendant is still reachable by its own name
	got, ok = Find(addons, "hive-ingestor-rfc")
	if !ok || got.ID != "io.github.hive-agi:hive-ingestor-rfc" {
		t.Fatalf("descendant lookup broke: %v %q", ok, got.ID)
	}
	// a full id still wins outright
	got, ok = Find(addons, "io.github.hive-agi:hive-ingestor-rfc")
	if !ok || got.ID != "io.github.hive-agi:hive-ingestor-rfc" {
		t.Fatalf("exact id lookup broke: %v %q", ok, got.ID)
	}
}
