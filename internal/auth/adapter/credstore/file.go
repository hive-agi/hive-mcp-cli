package credstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/port"
)

// fileBackend is the fallback for machines with no secret service: servers,
// CI, a VM over ssh. One JSON file, 0600, in a 0700 directory: plain text on
// disk, which is what gh and the cloud CLIs do in the same place, and said out
// loud by Kind so nobody mistakes it for the keyring.
type fileBackend struct{}

func (fileBackend) Name() string  { return "file" }
func (fileBackend) Priority() int { return 100 }

func (fileBackend) Open(configDir string) (port.CredentialStore, error) {
	return NewFile(filepath.Join(configDir, "credentials.json")), nil
}

// NewFile is the file store at an explicit path.
func NewFile(path string) port.CredentialStore { return &fileStore{path: path} }

type fileStore struct {
	path string
	mu   sync.Mutex
}

func (f *fileStore) load() (map[string]string, error) {
	m := map[string]string{}
	b, err := os.ReadFile(f.path)
	if os.IsNotExist(err) || (err == nil && len(b) == 0) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	return m, json.Unmarshal(b, &m)
}

func (f *fileStore) save(m map[string]string) error {
	if len(m) == 0 {
		if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
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
		return "", domain.ErrNotFound
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
	return f.save(m)
}

func (f *fileStore) Kind() string { return "file " + f.path + " (mode 0600; no system keyring found)" }
