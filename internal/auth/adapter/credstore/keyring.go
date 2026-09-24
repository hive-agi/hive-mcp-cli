package credstore

import (
	"errors"
	"time"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/auth/port"
	"github.com/zalando/go-keyring"
)

const service = "hive-cli"

// keyringBackend is the OS secret store: Secret Service (GNOME Keyring,
// KWallet) on Linux, the Keychain on macOS, Credential Manager on Windows.
type keyringBackend struct{}

func (keyringBackend) Name() string  { return "keyring" }
func (keyringBackend) Priority() int { return 10 }

// Open probes with a throwaway write. On a headless box the D-Bus call can
// hang or fail; a probe that times out means "not here", not "fail the login".
func (keyringBackend) Open(string) (port.CredentialStore, error) {
	done := make(chan error, 1)
	go func() {
		if err := keyring.Set(service, "probe", "ok"); err != nil {
			done <- err
			return
		}
		_ = keyring.Delete(service, "probe")
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			return nil, err
		}
		return keyringStore{}, nil
	case <-time.After(3 * time.Second):
		return nil, errors.New("system keyring did not answer within 3s")
	}
}

type keyringStore struct{}

func (keyringStore) Get(key string) (string, error) {
	v, err := keyring.Get(service, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", domain.ErrNotFound
	}
	return v, err
}

func (keyringStore) Set(key, value string) error { return keyring.Set(service, key, value) }

func (keyringStore) Delete(key string) error {
	if err := keyring.Delete(service, key); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

func (keyringStore) Kind() string { return "system keyring" }
