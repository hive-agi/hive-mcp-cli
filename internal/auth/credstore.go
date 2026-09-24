// Package auth signs the CLI in to the hive account, the way gh, tailscale and
// claude do: a browser round-trip or a device code, never a password typed
// into the terminal, and credentials kept where the OS keeps secrets.
package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zalando/go-keyring"
)

// ErrNotFound is a credential that was never stored, or was removed.
var ErrNotFound = errors.New("credential not found")

// Store holds secrets by key. Two implementations exist and the difference
// matters to the user, so Kind names which one is in use.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	Kind() string
}

const keyringService = "hive-cli"

// keyringStore is the OS secret store: Secret Service (GNOME Keyring,
// KWallet) on Linux, the Keychain on macOS, Credential Manager on Windows.
type keyringStore struct{}

func (keyringStore) Get(key string) (string, error) {
	v, err := keyring.Get(keyringService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}
func (keyringStore) Set(key, value string) error { return keyring.Set(keyringService, key, value) }
func (keyringStore) Delete(key string) error {
	err := keyring.Delete(keyringService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
func (keyringStore) Kind() string { return "system keyring" }

// fileStore is the fallback for machines with no secret service: servers, CI,
// a VM reached over ssh. One JSON file, 0600, in a 0700 directory. Plain text
// on disk, which is what gh and the cloud CLIs do in the same situation, and
// said out loud by Kind so nobody mistakes it for the keyring.
type fileStore struct {
	path string
	mu   sync.Mutex
}

func (f *fileStore) load() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if len(b) == 0 {
		return m, nil
	}
	return m, json.Unmarshal(b, &m)
}

func (f *fileStore) save(m map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func (f *fileStore) Get(key string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return "", err
	}
	v, ok := m[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (f *fileStore) Set(key, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	m[key] = value
	return f.save(m)
}

func (f *fileStore) Delete(key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := m[key]; !ok {
		return nil
	}
	delete(m, key)
	if len(m) == 0 {
		if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return f.save(m)
}

func (f *fileStore) Kind() string { return "file " + f.path + " (mode 0600; no system keyring found)" }

// ConfigDir is where the CLI keeps its state: $XDG_CONFIG_HOME/hive, else
// ~/.config/hive. HIVE_CONFIG_DIR overrides.
func ConfigDir() string {
	if d := os.Getenv("HIVE_CONFIG_DIR"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "hive")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "hive")
}

// NewFileStore is the file backend at an explicit path.
func NewFileStore(path string) Store { return &fileStore{path: path} }

// OpenStore picks the backend. HIVE_CREDENTIAL_STORE=file|keyring forces one.
// Otherwise the keyring is probed with a throwaway write: on a headless box the
// D-Bus call can hang or fail, and a probe that times out means "use the file",
// not "fail the login".
func OpenStore() (Store, error) {
	file := &fileStore{path: filepath.Join(ConfigDir(), "credentials.json")}
	switch os.Getenv("HIVE_CREDENTIAL_STORE") {
	case "file":
		return file, nil
	case "keyring":
		if err := probeKeyring(); err != nil {
			return nil, fmt.Errorf("HIVE_CREDENTIAL_STORE=keyring but the system keyring is unusable: %w", err)
		}
		return keyringStore{}, nil
	}
	if probeKeyring() == nil {
		return keyringStore{}, nil
	}
	return file, nil
}

func probeKeyring() error {
	done := make(chan error, 1)
	go func() {
		const k = "probe"
		if err := keyring.Set(keyringService, k, "ok"); err != nil {
			done <- err
			return
		}
		_ = keyring.Delete(keyringService, k)
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("system keyring did not answer within 3s")
	}
}
