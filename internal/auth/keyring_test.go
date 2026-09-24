package auth

import (
	"os"
	"testing"
)

// Talks to the REAL system keyring, so it only runs when asked:
// HIVE_TEST_KEYRING=1 go test ./internal/auth -run Keyring
func TestKeyringRoundTrip(t *testing.T) {
	if os.Getenv("HIVE_TEST_KEYRING") != "1" {
		t.Skip("set HIVE_TEST_KEYRING=1 to exercise the system keyring")
	}
	if err := probeKeyring(); err != nil {
		t.Fatalf("keyring unusable here: %v", err)
	}
	s := keyringStore{}
	const k = "test-roundtrip"
	if err := s.Set(k, "v1"); err != nil {
		t.Fatal(err)
	}
	defer s.Delete(k)
	if v, err := s.Get(k); err != nil || v != "v1" {
		t.Fatalf("got %q %v", v, err)
	}
	if err := s.Delete(k); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(k); err != ErrNotFound {
		t.Fatalf("after delete: %v", err)
	}
}
