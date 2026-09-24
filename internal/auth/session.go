package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Session is what `hive login` leaves behind. The secret parts (tokens) live
// in the Store; the rest is in a plain state file, so `hive auth status` can
// say who is signed in without touching the keyring.
type Session struct {
	Issuer   string    `json:"issuer"`
	ClientID string    `json:"client_id"`
	Email    string    `json:"email,omitempty"`
	Plan     string    `json:"plan,omitempty"`
	SignedIn time.Time `json:"signed_in"`
	// ArtifactTokenID is the store's id for the hv_live_ token this machine
	// minted at login, so `hive logout` revokes that one and no other.
	ArtifactTokenID string `json:"artifact_token_id,omitempty"`
	Store           string `json:"credential_store"`
}

const (
	keyToken    = "oidc-token"     // JSON Token: access + refresh
	keyArtifact = "artifact-token" // the hv_live_ token, also in ~/.m2/settings.xml
)

func statePath() string { return filepath.Join(ConfigDir(), "session.json") }

// LoadSession reads the state file. A missing one means signed out.
func LoadSession() (*Session, error) {
	b, err := os.ReadFile(statePath())
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var s Session
	return &s, json.Unmarshal(b, &s)
}

func saveSession(s *Session) error {
	if err := os.MkdirAll(ConfigDir(), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(statePath(), b, 0o600)
}

// Save records a completed sign-in.
func Save(st Store, s *Session, tok *Token, artifactToken string) error {
	b, _ := json.Marshal(tok)
	if err := st.Set(keyToken, string(b)); err != nil {
		return err
	}
	if artifactToken != "" {
		if err := st.Set(keyArtifact, artifactToken); err != nil {
			return err
		}
	}
	s.Store = st.Kind()
	return saveSession(s)
}

// ArtifactToken is the hv_live_ token minted at login, if any.
func ArtifactToken(st Store) (string, error) { return st.Get(keyArtifact) }

// AccessToken returns a valid access token for the signed-in user, refreshing
// (and re-storing) it when it has expired. ErrNotFound means "run hive login".
func AccessToken(ctx context.Context) (string, error) {
	sess, err := LoadSession()
	if err != nil {
		return "", err
	}
	st, err := OpenStore()
	if err != nil {
		return "", err
	}
	raw, err := st.Get(keyToken)
	if err != nil {
		return "", err
	}
	var tok Token
	if err := json.Unmarshal([]byte(raw), &tok); err != nil {
		return "", err
	}
	if tok.Valid() {
		return tok.AccessToken, nil
	}
	if tok.RefreshToken == "" {
		return "", errors.New("session expired: run hive login")
	}
	c := NewClient()
	c.Issuer, c.ClientID = sess.Issuer, sess.ClientID
	fresh, err := c.Refresh(ctx, tok.RefreshToken)
	if err != nil {
		return "", errors.New("session expired or revoked (" + err.Error() + "): run hive login")
	}
	b, _ := json.Marshal(fresh)
	_ = st.Set(keyToken, string(b))
	return fresh.AccessToken, nil
}

// Clear ends the local session: revokes the refresh token at the issuer (best
// effort), then removes every stored secret and the state file.
func Clear(ctx context.Context) (*Session, error) {
	sess, _ := LoadSession()
	st, err := OpenStore()
	if err != nil {
		return sess, err
	}
	if raw, err := st.Get(keyToken); err == nil && sess != nil {
		var tok Token
		if json.Unmarshal([]byte(raw), &tok) == nil && tok.RefreshToken != "" {
			c := NewClient()
			c.Issuer, c.ClientID = sess.Issuer, sess.ClientID
			_ = c.Revoke(ctx, tok.RefreshToken)
		}
	}
	_ = st.Delete(keyToken)
	_ = st.Delete(keyArtifact)
	if err := os.Remove(statePath()); err != nil && !os.IsNotExist(err) {
		return sess, err
	}
	return sess, nil
}
