// Package storeapi is a BOUNDARY adapter: port.Accounts over the hive-store
// HTTP API, and port.MavenSettings over ~/.m2/settings.xml. It translates the
// store package's wire shapes into auth's domain, so neither side knows the
// other's types.
package storeapi

import (
	"context"

	"github.com/hive-agi/hive-mcp-cli/internal/auth/domain"
	"github.com/hive-agi/hive-mcp-cli/internal/store"
)

// Accounts implements port.Accounts.
type Accounts struct{ BaseURL string }

func (a Accounts) client(access string) *store.Client {
	c := store.New()
	if a.BaseURL != "" {
		c.BaseURL = a.BaseURL
	}
	c.Token = access
	return c
}

func (a Accounts) Me(ctx context.Context, access string) (domain.Account, error) {
	me, err := a.client(access).Me(ctx)
	if err != nil {
		return domain.Account{}, err
	}
	acct := domain.Account{Email: me.Email}
	if s := me.Subscription; s != nil {
		acct.Plan = &domain.Plan{Name: s.Plan, Status: s.Status, Entitles: s.Entitles}
	}
	return acct, nil
}

func (a Accounts) Mint(ctx context.Context, access string, label domain.MachineLabel) (domain.ArtifactToken, error) {
	t, err := a.client(access).MintToken(ctx, string(label))
	if err != nil {
		return domain.ArtifactToken{}, err
	}
	return domain.ArtifactToken{ID: t.ID, Prefix: t.Prefix, Secret: t.Secret}, nil
}

func (a Accounts) RevokeArtifact(ctx context.Context, access, id string) error {
	return a.client(access).RevokeToken(ctx, id)
}

// Maven implements port.MavenSettings on the store's settings.xml upsert.
type Maven struct{}

func (Maven) Path() string { return store.SettingsPath() }

func (Maven) Write(secret string) (string, error) {
	return store.WriteSettings(store.SettingsPath(), store.RepoID, secret)
}

func (Maven) RemoveIfHolds(secret string) (bool, error) {
	return store.RemoveServerIfToken(store.SettingsPath(), store.RepoID, secret)
}
