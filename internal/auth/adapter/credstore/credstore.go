// Package credstore is a BOUNDARY adapter: port.CredentialStore backends and
// the registry that picks one. The set is OPEN: a backend (pass, 1Password,
// a TPM) is a Backend plus a Register call, and the selection follows (OCP).
package credstore

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/port"
)

// Backend is one way of keeping secrets.
type Backend interface {
	Name() string // what HIVE_CREDENTIAL_STORE selects it by
	// Priority orders automatic selection: lower is tried first.
	Priority() int
	// Open returns a usable store, or an error saying why this machine cannot
	// use the backend (no D-Bus, a locked keychain, ...).
	Open(configDir string) (port.CredentialStore, error)
}

var (
	mu       sync.RWMutex
	backends = map[string]Backend{}
)

// Register adds a backend, replacing any of the same name.
func Register(b Backend) {
	mu.Lock()
	defer mu.Unlock()
	backends[b.Name()] = b
}

func init() {
	Register(keyringBackend{})
	Register(fileBackend{})
}

// Open picks the backend: HIVE_CREDENTIAL_STORE names one explicitly;
// otherwise the first by priority that opens on this machine.
func Open(configDir string) (port.CredentialStore, error) {
	mu.RLock()
	defer mu.RUnlock()
	if name := os.Getenv("HIVE_CREDENTIAL_STORE"); name != "" {
		b, ok := backends[name]
		if !ok {
			return nil, fmt.Errorf("HIVE_CREDENTIAL_STORE=%s: no such backend (have %v)", name, names())
		}
		s, err := b.Open(configDir)
		if err != nil {
			return nil, fmt.Errorf("HIVE_CREDENTIAL_STORE=%s is unusable here: %w", name, err)
		}
		return s, nil
	}
	ordered := make([]Backend, 0, len(backends))
	for _, b := range backends {
		ordered = append(ordered, b)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Priority() < ordered[j].Priority() })
	var errs []error
	for _, b := range ordered {
		s, err := b.Open(configDir)
		if err == nil {
			return s, nil
		}
		errs = append(errs, fmt.Errorf("%s: %w", b.Name(), err))
	}
	return nil, errors.Join(errs...)
}

func names() []string {
	out := make([]string, 0, len(backends))
	for n := range backends {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
