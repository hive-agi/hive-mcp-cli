// Package auth holds only this test: the gate that keeps sign-in stratified.
//
// Sign-in is CPPB, one package per stratum:
//
//	domain    CORE      value objects                       std's pure corner only
//	policy    PROMOTE   every decision, pure                 + domain
//	port      PROTOCOL  the seams, small interfaces          + domain
//	pipeline  PIPELINE  use cases over ports, flow registry  + domain, policy, port; no I/O
//	adapter/* BOUNDARY  effects: HTTP, keyring, files, OS    never the pipeline, never each other
//
// The rule is read off the SOURCE (each package's import set), not asserted in
// a comment. Each rule is also run against a synthetic violating import, so a
// gate that stopped checking fails rather than passes.
package auth

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const mod = "github.com/hive-agi/hive-mcp-cli/internal/"

// rule judges one package's imports; it returns the offending ones.
type rule func(imports []string) []string

func only(allowed ...string) rule {
	set := map[string]bool{}
	for _, a := range allowed {
		set[a] = true
	}
	return func(imports []string) (bad []string) {
		for _, i := range imports {
			if !set[i] {
				bad = append(bad, i)
			}
		}
		return bad
	}
}

func none(forbiddenPrefixes ...string) rule {
	return func(imports []string) (bad []string) {
		for _, i := range imports {
			for _, p := range forbiddenPrefixes {
				if i == p || strings.HasPrefix(i, p+"/") {
					bad = append(bad, i)
				}
			}
		}
		return bad
	}
}

var (
	domainPkg   = mod + "auth/domain"
	policyPkg   = mod + "auth/policy"
	portPkg     = mod + "auth/port"
	pipelinePkg = mod + "auth/pipeline"
	adapterPkg  = mod + "auth/adapter"
)

// strata maps a package directory (relative to internal/) to its rule, and a
// violating import the rule must reject.
var strata = []struct {
	dir     string
	rule    rule
	violate string
}{
	{"auth/domain", only("errors", "strings", "time"), "os"},
	{"auth/policy", only("context", "crypto/sha256", "encoding/base64", "errors", "fmt", "net/url", "strings", "time", domainPkg), "net/http"},
	{"auth/port", only("context", "time", domainPkg), policyPkg},
	{"auth/pipeline", only("context", "encoding/json", "errors", "fmt", "sort", "sync", domainPkg, policyPkg, portPkg), adapterPkg + "/oidc"},
	{"auth/adapter/oidc", none(pipelinePkg, adapterPkg), pipelinePkg},
	{"auth/adapter/loopback", none(pipelinePkg, adapterPkg), adapterPkg + "/oidc"},
	{"auth/adapter/credstore", none(pipelinePkg, adapterPkg), pipelinePkg},
	{"auth/adapter/system", none(pipelinePkg, adapterPkg), pipelinePkg},
	{"auth/adapter/storeapi", none(pipelinePkg, adapterPkg), adapterPkg + "/credstore"},
	// The store is a LOWER stratum than auth: it must not reach up into it.
	{"store", none(mod + "auth"), mod + "auth/pipeline"},
}

func importsOf(t *testing.T, dir string) []string {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", n, err)
		}
		for _, imp := range f.Imports {
			seen[strings.Trim(imp.Path.Value, `"`)] = true
		}
	}
	out := make([]string, 0, len(seen))
	for i := range seen {
		out = append(out, i)
	}
	sort.Strings(out)
	return out
}

func TestStrata(t *testing.T) {
	for _, s := range strata {
		imports := importsOf(t, filepath.Join("..", s.dir))
		if bad := s.rule(imports); len(bad) > 0 {
			t.Errorf("%s crosses its stratum: %v", s.dir, bad)
		}
		// The gate must bite: the same rule rejects a known violation.
		if bad := s.rule(append(imports, s.violate)); len(bad) == 0 {
			t.Errorf("%s: the rule accepted %q; the gate has stopped checking", s.dir, s.violate)
		}
	}
}

// Every stratum directory exists and has code: a renamed package must not
// silently drop out of the gate.
func TestStrataCoverEveryAuthPackage(t *testing.T) {
	covered := map[string]bool{}
	for _, s := range strata {
		covered[s.dir] = true
	}
	filepath.WalkDir(".", func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() || p == "." {
			return nil
		}
		if files, _ := filepath.Glob(filepath.Join(p, "*.go")); len(files) > 0 && !covered["auth/"+p] {
			t.Errorf("auth/%s has code but no stratum rule", p)
		}
		return nil
	})
}
