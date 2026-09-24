package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// MintedToken is POST /api/tokens: the plaintext secret appears here once and
// never again; the store keeps only its hash.
type MintedToken struct {
	Secret string `json:"secret"`
	Prefix string `json:"prefix"`
	ID     string `json:"id"`
}

func (c *Client) send(ctx context.Context, method, path string, body, into any) error {
	if c.Token == "" {
		return fmt.Errorf("not signed in: run hive login")
	}
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("%s %s: the store did not accept the sign-in (401); run hive login again", method, path)
	case resp.StatusCode == http.StatusNotFound && method == http.MethodDelete:
		return nil // already revoked, or never ours: the outcome logout wants either way
	case resp.StatusCode >= 400:
		return fmt.Errorf("%s %s%s: store answered %s", method, c.BaseURL, path, resp.Status)
	}
	if into == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(into)
}

// MintToken creates an artifact token for the signed-in account.
func (c *Client) MintToken(ctx context.Context, label string) (*MintedToken, error) {
	var t MintedToken
	if err := c.send(ctx, http.MethodPost, "/api/tokens", map[string]string{"label": label}, &t); err != nil {
		return nil, err
	}
	if t.Secret == "" || t.ID == "" {
		return nil, fmt.Errorf("the store minted a token but returned no secret or id")
	}
	return &t, nil
}

// RevokeToken revokes one artifact token by id. Revoking one that is already
// gone is not an error.
func (c *Client) RevokeToken(ctx context.Context, id string) error {
	return c.send(ctx, http.MethodDelete, "/api/tokens/"+id, nil, nil)
}
