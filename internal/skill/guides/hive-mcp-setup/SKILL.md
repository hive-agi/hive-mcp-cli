---
name: hive-mcp-setup
description: Set up the hive-mcp harness on this machine, end to end — the batteries-included FOSS build, or a licensed build for someone who already has a store token. Use when the user says "help me set up hive-mcp", "set up the hive harness locally", "I have a key / a token / a subscription", "install the FOSS build", "set up hive-mcp with my subscription", or when `hive doctor` reports a broken install and they want it repaired. Also covers verifying the install, registering it with Claude Code, and the catchup/wrap rituals a first session needs.
---

# Setting up hive-mcp

hive-mcp is a **host**: a Clojure runtime that addons mount into, exposed to Claude
Code over MCP. Setting it up means four things, in this order:

1. the host on disk and its prerequisites,
2. the addons it loads,
3. registration with Claude Code,
4. proof that all three worked.

There are two builds. **Ask which one before touching anything** — the addon step
differs completely, and everything else is shared.

| | FOSS build | Licensed build |
|---|---|---|
| Addons | `starter.deps.edn`, published on Clojars | the FOSS set **plus** subscription artifacts from the store gateway |
| Credential | none | an artifact token, `hv_live_…` |
| Gets you | memory, KG, kanban, swarm, session rituals, code intelligence | the above plus carto, hive-shape, hive-dsl, hive-test, hive-schemas … |

If the user said "I have a key", they mean the licensed build — go to
[Licensed build](#licensed-build), but do part 1 first: **the token adds addons to a
host that must already exist.** A subscriber with no hive-mcp on disk has nothing for
the token to feed.

---

## Part 1 — the host (both builds)

### The fast path

```bash
hive detect     # what is already here
hive setup      # clone, prerequisites, vector store, register with Claude Code
hive doctor     # every check
```

`hive setup` is idempotent: a step whose check already passes is skipped, and a
failing step rolls back what it did. Re-running it is always safe, so prefer it over
hand-repair when `hive doctor` is unhappy.

If the `hive` CLI is not installed:

```bash
curl -fsSL https://hive-mcp.com/install.sh | sh
```

…or, with a Go toolchain present:

```bash
go install github.com/hive-agi/hive-mcp-cli/cmd/hive@latest
```

### What it needs

| Requirement | Version | Why |
|---|---|---|
| Java | 21 | the host is a JVM process; CI and the image both run 21 |
| Clojure CLI | 1.12+ | the host runs from source |
| Docker | recent | Chroma (vectors) and the clojure-lsp sidecar |
| Git | 2+ | clones the host and any git-sourced addon |
| Claude Code | latest | the MCP client |

Optional, each degrading to "that one feature is dormant" rather than a failed boot:
Ollama with `nomic-embed-text` for semantic search over memory; tmux with Python 3 +
libtmux for the headless vessel; Emacs 28.1+ for the Emacs vessel.

`HIVE_SKIP_COMPOSE=1` if Chroma and the sidecar already run elsewhere.

### Doing it by hand

When the CLI is unavailable, the same four steps:

```bash
git clone https://github.com/hive-agi/hive-mcp.git ~/hive-mcp
cd ~/hive-mcp
bin/hive-mcp-foss                       # starts services, merges the starter pack, boots on nREPL
claude mcp add hive -- "$HOME/hive-mcp/bin/hive-mcp-foss"
```

`bin/hive-mcp-foss` merges [`starter.deps.edn`](https://github.com/hive-agi/hive-mcp/blob/main/starter.deps.edn)
over `deps.edn` at boot — that file **is** the FOSS addon set. `HIVE_STARTER=0` boots
the bare core instead, which is a diagnostic, not a normal setup.

A personal overlay goes in a gitignored `local.deps.edn` beside it; the launcher
merges that too. That is also where a licensed build's coordinates land.

---

## Part 2a — FOSS build

Nothing further. `starter.deps.edn` is already merged by the launcher, every
coordinate in it is on Clojars, and no registry, VPN or credential is involved.

An addon whose host tool is missing (no Emacs daemon, no tmux) **degrades with a
logged reason and the boot still completes**. Do not treat such a line as a failure;
check `hive doctor` for the verdict.

---

## Part 2b — licensed build {#licensed-build}

There are **two different credentials** here and confusing them is the most common
failure:

| Credential | Where it comes from | Where it goes | What it opens |
|---|---|---|---|
| **Artifact token** `hv_live_…` | minted in the store dashboard, shown once | `~/.m2/settings.xml` | the Maven gateway, `store.hive-mcp.com/maven` |
| **Store session token** (OIDC) | signing in at `auth.hive-mcp.com` | `HIVE_STORE_TOKEN` | the store API — `/api/me`, entitlements, `hive addon --owned` |

The artifact token is the one that makes builds resolve. When the user says "I have a
key", ask which of the two they hold; if they minted it in the dashboard, it is the
artifact token.

### 1. Point the project at the gateway

In the project's `deps.edn` — **not** in `~/.clojure/deps.edn`:

```clojure
{:mvn/repos {"hive-store" {:url "https://store.hive-mcp.com/maven"}}
 :deps {io.github.hive-agi/hive-carto {:mvn/version "RELEASE"}}}
```

The repo key `hive-store` is load-bearing. It must match the `<id>` in the next step
exactly, or Maven never attaches the credential and the gateway sees an anonymous
request.

`hive addon show <id>` prints the coordinate for any addon; `hive addon search`
finds one by description. `https://store.hive-mcp.com/api/versions` answers what the
whole shelf resolves to, publicly and with no token — that is what a build asks
before it bumps a pin.

### 2. Give Maven the credential

In `~/.m2/settings.xml`:

```xml
<settings>
  <servers>
    <server>
      <id>hive-store</id>
      <username>hv_live_…</username>
      <password>hv_live_…</password>
    </server>
  </servers>
</settings>
```

The token goes in **both** fields. Aether sends either position depending on the
request, and a mismatch reads as anonymous, which the gateway answers with 401.

The token never goes in `deps.edn` and never in a repository.

### 3. Prove it resolves

```bash
clj -Sforce -Sdeps '{:mvn/repos {"hive-store" {:url "https://store.hive-mcp.com/maven"}} :deps {io.github.hive-agi/hive-carto {:mvn/version "RELEASE"}}}' -e "(println :resolved)"
```

### 4. Load them into the host

Licensed addons mount the same way FOSS ones do — put their coordinates in
`~/hive-mcp/local.deps.edn` (gitignored, merged by the launcher) and restart the
host. `hive addon status` reports what actually mounted.

### When a licensed resolve fails

Three causes, in the order worth checking:

1. **The ids disagree.** The `:mvn/repos` key and the settings.xml `<id>` are both
   `hive-store`, or the credential is never attached.
2. **A global `deps.edn` shadows the project's.** If `~/.clojure/deps.edn` already
   defines `hive-store` pointing elsewhere, it wins, and the failure looks like a
   permissions problem rather than a configuration one. Check there first.
3. **`http://`.** Both aether and tools.deps reject a plain-HTTP repository outright.

Revoking a token in the dashboard stops it resolving immediately; there is no cache
to wait out.

---

## Part 3 — register with Claude Code

```bash
claude mcp add hive -- "$HOME/hive-mcp/bin/hive-mcp-foss"
```

`hive setup` does this for you. Use `--scope user` to make it available in every
project rather than the current one.

---

## Part 4 — prove it

```bash
hive doctor                      # versions, services, env, registration, nREPL on 7910
claude mcp list | grep hive      # hive: …/bin/hive-mcp-foss
```

`hive doctor --fix` attempts repairs for the fixable checks.

Then, inside Claude Code in a real project, the two rituals — plain requests, not
slash commands; the model reaches for the tools itself:

```
hive `project workflow catchup` using pwd as dir
```

```
make memories on all learnings this session, kg connect them,
sync kanban, and `workflow wrap`
```

Catchup is what a new session starts with; wrap is what makes the next catchup worth
running. A setup that boots but is never wrapped stores nothing.

---

## Diagnosing a bad install

Work outward from the process:

| Symptom | Look at |
|---|---|
| `claude mcp list` has no `hive` | registration — re-run `hive setup`, or `claude mcp add` by hand |
| registered but tools missing | the host did not boot: run `bin/hive-mcp-foss` in a terminal and read the log |
| host boots, one addon absent | its host tool is missing — the log names the reason; `hive addon status` confirms |
| memory works, semantic search does not | Ollama or the embedding model: `ollama pull nomic-embed-text` |
| licensed addon will not resolve | the three causes above, in that order |
| everything green, nothing remembered | no wrap has ever run — see Part 4 |

Full reference, always current: <https://github.com/hive-agi/hive-mcp/wiki>.
The store's own setup recipe, with the user's real coordinates substituted in:
<https://store.hive-mcp.com/setup>.
