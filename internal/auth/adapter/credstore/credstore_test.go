package credstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/port"
)

func TestFileStoreIsPrivateAndRemovesItselfWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hive", "credentials.json")
	s := NewFile(path)
	if _, err := s.Get("x"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("empty store: %v", err)
	}
	if err := s.Set("x", "secret"); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("file mode %v", info.Mode().Perm())
	}
	if dir, _ := os.Stat(filepath.Dir(path)); dir.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v", dir.Mode().Perm())
	}
	s.Delete("x")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an empty store should leave no file behind")
	}
}

// memBackend is a backend added by REGISTRATION alone: the OCP claim for the
// registry, and proof that priority, not code order, picks the backend.
type memBackend struct{ fail bool }

func (memBackend) Name() string  { return "memory-test" }
func (memBackend) Priority() int { return 1 }
func (b memBackend) Open(string) (port.CredentialStore, error) {
	if b.fail {
		return nil, errors.New("unavailable here")
	}
	return NewFile(filepath.Join(os.TempDir(), "unused")), nil
}

func TestRegistryPicksByPriorityAndFallsThrough(t *testing.T) {
	t.Setenv("HIVE_CREDENTIAL_STORE", "")
	defer func() { mu.Lock(); delete(backends, "memory-test"); mu.Unlock() }()

	Register(memBackend{})
	s, err := Open(t.TempDir())
	if err != nil || s.Kind() == "system keyring" {
		t.Fatalf("the registered priority-1 backend should win: %v %v", s, err)
	}
	Register(memBackend{fail: true}) // same name: replaces
	t.Setenv("HIVE_CREDENTIAL_STORE", "file")
	s, err = Open(t.TempDir())
	if err != nil || s.Kind()[:4] != "file" {
		t.Fatalf("explicit file: %v %v", s, err)
	}
	t.Setenv("HIVE_CREDENTIAL_STORE", "memory-test")
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("an explicitly chosen backend that cannot open must say so, not fall back")
	}
	t.Setenv("HIVE_CREDENTIAL_STORE", "nope")
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("unknown backend name must fail")
	}
}

// Talks to the REAL system keyring, so it only runs when asked:
// HIVE_TEST_KEYRING=1 go test ./internal/auth/adapter/credstore -run Keyring
func TestKeyringRoundTrip(t *testing.T) {
	if os.Getenv("HIVE_TEST_KEYRING") != "1" {
		t.Skip("set HIVE_TEST_KEYRING=1 to exercise the system keyring")
	}
	s, err := keyringBackend{}.Open("")
	if err != nil {
		t.Fatalf("keyring unusable here: %v", err)
	}
	const k = "test-roundtrip"
	defer s.Delete(k)
	if err := s.Set(k, "v1"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get(k); err != nil || v != "v1" {
		t.Fatalf("got %q %v", v, err)
	}
	s.Delete(k)
	if _, err := s.Get(k); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}
