# hive-mcp-cli

The `hive` command: sets up [hive-mcp](https://github.com/hive-agi/hive-mcp) on your
machine and signs you in to your hive account. hive-mcp gives Claude Code persistent
memory, a knowledge graph, kanban and swarm coordination.

## Quick start

```bash
curl -fsSL https://hive-mcp.com/install.sh | sh    # 1. the hive CLI (10 seconds)
hive setup                                         # 2. everything else (about 5 minutes)
```

Open a **new terminal**, start `claude` in any project, and hive is there. That is the
free (FOSS) build, and it needs no account.

**Subscribers** add one more step:

```bash
hive login                  # opens your browser; sign in; you are done
hive addon add hive-carto   # any addon on your plan; then restart Claude Code
```

No keys to copy, no XML to paste.

<details>
<summary>What <code>hive setup</code> does, and what it needs</summary>

It runs on Linux (apt) and macOS (Homebrew). It asks for `sudo` once, to install packages.

1. **Clones** hive-mcp to `~/hive-mcp` (`HIVE_MCP_DIR` overrides).
2. **Exports** `HIVE_MCP_DIR` in your shell rc file.
3. **Installs** what is missing: Java 21, the Clojure CLI, Docker with compose, and Git.
   On Linux it adds you to the `docker` group. A new terminal picks that up.
4. **Resolves** the host's dependencies: the core plus the FOSS starter pack.
5. **Starts** Chroma, the vector store, with Docker.
6. **Pulls** the embedding model if [Ollama](https://ollama.com) is installed, and skips this otherwise.
7. **Registers** hive with Claude Code for every project:
   `claude mcp add --scope user hive -- ~/hive-mcp/bin/hive-mcp-foss`.

Every step checks first, so re-running `hive setup` is always safe. The first time
Claude Code starts hive, the host needs about a minute to boot.

You need [Claude Code](https://claude.ai/download). These are optional:

- **Ollama**, for semantic search over memory.
- **Emacs 28.1+**, for the Emacs vessel (`hive setup --emacs`).
- **tmux** with Python's `libtmux`, for headless lings.
- **clojure-lsp, clj-kondo and scc**, for the code-intelligence addons.

Each missing one turns off that one feature. It never fails the setup.
</details>

## Signing in

`hive login` works like `gh auth login`:

- **With a browser**, it opens the sign-in page and waits for the redirect back to the
  CLI. You sign in the way you do on the web, including a second factor if your account
  has one.
- **Over ssh, or on a machine without a display**, it prints a one-time code instead:

  ```
    First copy your one-time code: WDJB-MJHT
    Then open:                     https://auth.hive-mcp.com/realms/hive/device
  ```

  Open that URL on your laptop or phone, enter the code, and approve. The terminal
  continues. `hive login --device` forces this mode.

Once signed in, the CLI:

1. keeps your session in the **system keyring**: GNOME Keyring or KWallet, the macOS
   Keychain, or the Windows Credential Manager. A machine without a keyring uses
   `~/.config/hive/credentials.json` instead, mode 0600, and `hive login` says so.
2. creates an **artifact token for this machine only**, named after its hostname.
   It writes the token to `~/.m2/settings.xml`, which is what lets Maven download
   paid addons. The token appears in your dashboard, so you can revoke it there.
3. prints who you are signed in as and what your plan includes.

```bash
hive auth status   # who is signed in, where the session is stored, what is set up
hive auth token    # a current access token, for scripts that call the store API
hive logout        # revokes this machine's token, removes it, ends the session
```

In CI, where no browser is available, pass a token you created in the dashboard:
`printf %s "$HIVE_ARTIFACT_TOKEN" | hive login --with-token`. Setting
`HIVE_STORE_TOKEN` overrides the session for store API calls.

## Adding addons

```bash
hive addon search            # the whole shelf; `hive addon search graph` narrows it
hive addon show hive-carto   # what it does and needs, and whether your plan includes it
hive addon add hive-carto    # load it into your host
```

`addon add` writes the coordinate to `~/hive-mcp/local.deps.edn`, your personal
overlay. It is gitignored, and the launcher merges it over the starter pack at every
boot. If you already wrote that file yourself, the command leaves it alone and prints
the lines to add. After adding, restart Claude Code, or run `/mcp` and reconnect hive.

## Let Claude do it

The installer also teaches Claude Code the whole procedure. It installs two skills and
a small setup MCP server. In Claude Code you can then say:

> set up hive for me

> I have a hive subscription, set up the paid addons

Claude runs `hive setup`, `hive login` and `hive addon add` for you. Your password
and tokens never pass through the conversation: the sign-in happens in your browser.

## When something is wrong

```bash
hive doctor          # every check, with the fix for each failure
hive doctor --fix    # applies the fixes it can
```

| Symptom | Fix |
|---|---|
| `claude mcp list` shows no `hive` | re-run `hive setup` |
| hive is listed but `✘ Failed to connect` | run `~/hive-mcp/bin/hive-mcp-foss` in a terminal and read the log |
| `permission denied` on the docker socket | open a new terminal; setup added you to the `docker` group |
| a paid addon will not resolve | run `hive auth status`, then `hive login` again |
| `hive login` says the server does not recognise this CLI | run `hive store login` and paste a token from the [dashboard](https://store.hive-mcp.com/dashboard) |

## Verifying the installer

Every binary is checked against a signed `SHA256SUMS` before it is made executable,
and the installer prints which checks it managed. To read the script first, and check
it against a second origin:

```bash
curl -fsSL https://hive-mcp.com/install.sh -o install.sh
curl -fsSL https://hive-mcp.com/install.sh.sha256    # served from our cluster, not GitHub
sha256sum -c install.sh.sha256
less install.sh && sh install.sh
```

`sh -s -- --setup` runs `hive setup` in the same pass. It is opt-in, because a script
piped from the internet should not edit your shell config unless you ask it to.
[Verifying the install](https://docs.hive-mcp.com/Verifying-The-Install.html) explains
what each check proves.

With Go, instead of the installer:

```bash
go install github.com/hive-agi/hive-mcp-cli/cmd/hive@latest
go install github.com/hive-agi/hive-mcp-cli/cmd/hive-setup-mcp@latest
hive guide --install && claude mcp add hive-setup --scope user -- hive-setup-mcp
```

## Command reference

| Command | Does |
|---|---|
| `hive setup [--emacs]` | installs and registers everything; safe to re-run |
| `hive login [--device \| --with-token]` | signs in; creates this machine's artifact token |
| `hive logout` | revokes and forgets this machine's credentials |
| `hive auth status \| token` | shows who is signed in; prints an access token |
| `hive addon search \| show \| add \| coord` | browses the store and loads an addon |
| `hive addon skill [--owned] [--install]` | writes a Claude Code skill per addon |
| `hive addon new <id> [--extends <other>]` | scaffolds your own addon |
| `hive addon status` | shows what the host would mount, and why not |
| `hive doctor [--fix]` | runs health checks |
| `hive detect` | reports prerequisites and services, read-only |
| `hive guide [--install]` | prints or installs the setup skills |
| `hive store login [--check]` | pastes an artifact token by hand, or checks the wiring |

Environment variables:

| Variable | Sets |
|---|---|
| `HIVE_MCP_DIR` | where the host is checked out (`~/hive-mcp`) |
| `HIVE_STORE_URL` | the store (`https://store.hive-mcp.com`) |
| `HIVE_AUTH_ISSUER`, `HIVE_AUTH_CLIENT_ID` | the sign-in realm and client, for a dev realm |
| `HIVE_CREDENTIAL_STORE` | `keyring` or `file`, to force a backend |
| `HIVE_CONFIG_DIR` | where the session is kept (`~/.config/hive`) |

`hive-setup-mcp` exposes these commands as MCP tools (`hive_setup`, `hive_doctor`,
`hive_addon_*`, and so on). Its schemas are generated by
[Bonzai](https://github.com/rwxrob/bonzai) from each command's `Mcp` metadata.

## Development

```bash
go test ./...                        # unit tests
test/auth/e2e.sh                     # hive login/logout against a real Keycloak 26 (docker)
test/vm/customer-journey.sh --dev    # a customer's first run on a fresh Ubuntu VM
```

- `test/auth/e2e.sh` starts the Keycloak version production runs, with the `hive-cli`
  client configured as in `k8s-agi/terraform/keycloak-hive-realm/clients-cli.tf`. It
  also starts a stand-in store that enforces the real store's `aud` check. It then
  drives the real binary through both sign-in flows, token refresh, re-login and
  logout. A scripted browser fills in the Keycloak pages.
- `test/vm/customer-journey.sh` needs VirtualBox, but no root. It installs Claude
  Code, runs `install.sh` and `hive setup`, and checks `claude mcp list` from an
  unrelated project. It prints PASS or FAIL for each stage. `HIVE_MCP_LAUNCHER`
  swaps in a dev launcher, and `test/vm/hive-vm` drives the VM by hand.

### How sign-in is built

`internal/auth` is stratified by the house pattern (CPPB, DDD, SOLID). Each layer
is its own Go package, so the compiler enforces the layering:

| Package | Stratum | Holds |
|---|---|---|
| `auth/domain` | core | value objects: `Realm`, `Tokens`, `DeviceGrant`, `Account`, `ArtifactToken`, `Session` |
| `auth/policy` | promote (pure) | every decision: method choice, PKCE, poll verdicts, offline fallback, error classes |
| `auth/port` | protocol | small interfaces: identity provider, callback, browser, credential store, accounts, and so on |
| `auth/pipeline` | pipeline | the Login, Logout, Status and AccessToken use cases, plus the sign-in **flow registry** |
| `auth/adapter/*` | boundary | Keycloak HTTP, the loopback listener, the keyring/file **backend registry**, the store API |
| `internal/hive/wire.go` | composition root | which adapter fills which port |

Adding a sign-in method or a credential backend is a registration, never an edit.
`internal/auth/strata_test.go` reads every package's imports and fails the build
when a layer reaches past its stratum.

## License

MIT
