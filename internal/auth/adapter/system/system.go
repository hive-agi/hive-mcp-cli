// Package system is a BOUNDARY adapter for the machine itself: the browser,
// facts about the environment, the clock, randomness, and the session file.
package system

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
)

// ConfigDir is where the CLI keeps its state: HIVE_CONFIG_DIR, else
// $XDG_CONFIG_HOME/hive, else ~/.config/hive.
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

// Machine implements port.Machine from the process environment.
type Machine struct{}

func (Machine) Environment() domain.Environment {
	_, opener := exec.LookPath("xdg-open")
	return domain.Environment{
		OverSSH:         os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "",
		HasDisplay:      os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != "",
		BrowserOverride: os.Getenv("BROWSER") != "",
		DesktopOS:       runtime.GOOS == "darwin" || runtime.GOOS == "windows",
		HasOpener:       opener == nil,
	}
}

func (Machine) Hostname() string { h, _ := os.Hostname(); return h }

// Browser implements port.Browser. $BROWSER wins, as it does for gh and git.
type Browser struct{}

func (Browser) Open(url string) error {
	var cmd *exec.Cmd
	switch {
	case os.Getenv("BROWSER") != "":
		cmd = exec.Command(os.Getenv("BROWSER"), url)
	case runtime.GOOS == "darwin":
		cmd = exec.Command("open", url)
	case runtime.GOOS == "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		if _, err := exec.LookPath("xdg-open"); err != nil {
			return errors.New("no xdg-open")
		}
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// Clock implements port.Clock with real time.
type Clock struct{}

func (Clock) Now() time.Time { return time.Now() }

func (Clock) Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Entropy implements port.Entropy from crypto/rand.
type Entropy struct{}

func (Entropy) Token(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // no randomness means no safe sign-in; do not continue
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// SessionFile implements port.SessionRepo as JSON in the config dir. It holds
// no secret: those are in the credential store.
type SessionFile struct{ Dir string }

func (s SessionFile) path() string { return filepath.Join(s.Dir, "session.json") }

func (s SessionFile) Load() (*domain.Session, error) {
	b, err := os.ReadFile(s.path())
	if os.IsNotExist(err) {
		return nil, domain.ErrNotSignedIn
	}
	if err != nil {
		return nil, err
	}
	var sess domain.Session
	return &sess, json.Unmarshal(b, &sess)
}

func (s SessionFile) Save(sess domain.Session) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(), b, 0o600)
}

func (s SessionFile) Remove() error {
	if err := os.Remove(s.path()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
