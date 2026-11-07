---
name: hive-store
description: Use the hive store: browse the addon catalog, understand what an addon does and what it depends on, subscribe, sign a machine in with hive login, and make a licensed coordinate resolve. Use when the user asks what hive addons exist, what a subscription includes, how to buy or pay for one, how to mint or revoke a token, why a licensed artifact will not resolve, or what version an addon is on. Covers the public API endpoints an agent or a build can read without any credential.
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

`--owned` needs a signed-in account (`hive login`); without one the command errors
rather than quietly rendering nothing, because "I could not ask" is not "you own
nothing".

## Subscribing

1. **Subscribe** at <https://store.hive-mcp.com/pricing>. Monero straight to a wallet
   the store runs, or by card.
2. **Sign this machine in**: `hive login`. It opens the browser (or prints a one-time
   code on a machine without one), mints an artifact token named after the machine,
   and writes it where Maven reads it. You may run it for the user; nothing secret
   passes through the conversation.
3. **Load what they bought**: `hive addon add <id>`, then restart Claude Code.

`hive auth status` says who is signed in and whether the gateway accepts the token.
`hive logout` revokes this machine's token and forgets the session.

Without the CLI (CI, another build tool): mint a token in
<https://store.hive-mcp.com/dashboard> (shown **once**), and
<https://store.hive-mcp.com/setup> renders the exact `deps.edn` and `settings.xml`.

### The setup recipe, by hand

This is the whole recipe; `hive store login` writes exactly this.

In the project's `deps.edn` (not `~/.clojure/deps.edn`):

```clojure
{:mvn/repos {"hive-store" {:url "https://store.hive-mcp.com/maven"}}}
```

In `~/.m2/settings.xml`, mode 0600, the token in **both** fields:

```xml
<settings><servers><server>
  <id>hive-store</id>
  <username>hv_live_...</username>
  <password>hv_live_...</password>
</server></servers></settings>
```

The URL **must** be `https://`. Both aether and tools.deps refuse a plain `http://`
repository, so there is no "try it without TLS first" step; a dev store needs a
certificate too.

## The two credentials

`hive login` handles both. They still differ, and it matters when diagnosing:

| | Artifact token | Session |
|---|---|---|
| Looks like | `hv_live_…` | an OIDC access token, refreshed automatically |
| Comes from | `hive login` (one per machine), or the dashboard | `hive login` |
| Lives in | `~/.m2/settings.xml` | the system keyring; `HIVE_STORE_TOKEN` overrides |
| Authenticates | Basic auth to `/maven` | Bearer to `/api/me`, `/api/license`, `/api/tokens` |
| Used by | tools.deps, Maven, your build | `hive addon --owned`, `hive auth token` |

Revoking an artifact token in the dashboard stops it resolving immediately. There is
no cache to wait out. Each machine's token is listed there under its hostname.

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

6. **The token was revoked, expired, or the subscription lapsed.** Maven shows only
   the status (401, 402, 403). The gateway names the reason in the
   `x-hive-store-reason` header and in `WWW-Authenticate`; `hive store login --check`
   and `hive doctor` (Store Credential) read it and print the sentence, e.g. "this
   token was revoked; mint a new one ...". Run one of those before guessing.

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
