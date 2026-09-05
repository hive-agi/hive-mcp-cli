---
name: hive-store
description: Use the hive store: browse the addon catalog, understand what an addon does and what it depends on, subscribe, mint an artifact token, and make a licensed coordinate resolve. Use when the user asks what hive addons exist, what a subscription includes, how to buy or pay for one, how to mint or revoke a token, why a licensed artifact will not resolve, or what version an addon is on. Covers the public API endpoints an agent or a build can read without any credential.
---

# The hive store

`store.hive-mcp.com` is the storefront: catalog, subscriptions, tokens, and the Maven
gateway that checks them. Two things live behind it, what is **for sale** (the
catalog) and what **exists** (the versions shelf), and they are different
questions.

## Read it without a credential

Two endpoints are public on purpose, because a build deciding whether to bump has no
session:

```bash
curl -s https://store.hive-mcp.com/api/catalog    # what is for sale, with capabilities
curl -s https://store.hive-mcp.com/api/versions   # what every artifact resolves to
curl -s https://store.hive-mcp.com/api/versions/hive-carto
```

Prefer the CLI when it is installed. Same data, readable:

```bash
hive addon search carto          # free text over id, name, blurb, tags, capabilities
hive addon search --shelf code   # code | knowledge | agents | verification | editor | infrastructure
hive addon show hive-carto       # what it is, what it needs, what it opens
hive addon coord hive-carto      # just the deps.edn coordinate
hive addon status                # what is mounted in the host right now
```

`hive addon show` reports whether the coordinate **resolves today**. A preview addon
is described without pretending it can be fetched; do not hand someone a coordinate
whose status is not `available`.

## Render a skill for an addon

The CLI projects a Claude Code skill from the catalog, so a skill cannot claim
something the catalog does not:

```bash
hive addon skill hive-carto             # print one
hive addon skill --install              # write every addon's skill to ~/.claude/skills
hive addon skill --owned --install      # only what this account is entitled to
```

`--owned` needs `HIVE_STORE_TOKEN`; without it the command errors rather than
quietly rendering nothing, because "I could not ask" is not "you own nothing".

## Subscribing

1. **Sign in** at `auth.hive-mcp.com`, one realm shared by every hive surface.
2. **Subscribe** at <https://store.hive-mcp.com/pricing>. Monero straight to a wallet
   the store runs, or by card.
3. **Mint a token** in <https://store.hive-mcp.com/dashboard>. It is shown **once**.
4. **Wire it up**. <https://store.hive-mcp.com/setup> renders the exact `deps.edn`
   and `settings.xml` with this account's real coordinates substituted in.

Step 4 is the same recipe the `hive-mcp-setup` skill carries under "licensed build";
use that skill for the full machine setup, this one for the store itself.

## The two credentials

They are not interchangeable, and mixing them up is the usual failure:

| | Artifact token | Store session token |
|---|---|---|
| Looks like | `hv_live_…` | an OIDC access token |
| Comes from | the dashboard, shown once | signing in at `auth.hive-mcp.com` |
| Lives in | `~/.m2/settings.xml` | `HIVE_STORE_TOKEN` |
| Authenticates | Basic auth to `/maven` | Bearer to `/api/me`, `/api/license`, `/api/tokens` |
| Used by | tools.deps, Maven, your build | `hive addon --owned`, the dashboard |

Revoking an artifact token in the dashboard stops it resolving immediately. There is
no cache to wait out.

## Why a licensed coordinate will not resolve

In the order worth checking:

1. **The repo ids disagree.** The `:mvn/repos` key in `deps.edn` and the `<id>` in
   `settings.xml` must both be `hive-store`. If they differ, Maven never attaches the
   credential and the gateway sees an anonymous request, a 401 that reads like a
   permissions problem.
2. **The token is in one field only.** It goes in `<username>` *and* `<password>`;
   aether sends either position depending on the request.
3. **A global `deps.edn` shadows the project's.** `~/.clojure/deps.edn` defining
   `hive-store` elsewhere wins silently.
4. **Plain `http://`.** Both aether and tools.deps reject it outright.
5. **The addon is preview, not available.** `hive addon show <id>` says which.

Proof, in one command:

```bash
clj -Sforce -Sdeps '{:mvn/repos {"hive-store" {:url "https://store.hive-mcp.com/maven"}} :deps {io.github.hive-agi/hive-carto {:mvn/version "RELEASE"}}}' -e "(println :resolved)"
```

## Self-hosting or a dev store

`HIVE_STORE_URL` points the CLI at another deployment. The apex `hive-mcp.com` is
**not** the store: it is the landing page and answers 404 for every API path.

## Entitlement is decided once

Whether an addon may **mount** is decided by the licence gate inside the JVM that
mounts it, offline, from an Ed25519-signed licence. Neither the CLI nor the store API
is a second place that decision is made. An answer computed anywhere else is a
second licence decision, which is exactly what the gate exists to prevent.
