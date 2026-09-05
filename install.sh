#!/bin/sh
# hive-mcp installer.
#
#   curl -fsSL https://hive-mcp.com/install.sh | sh
#
# WHAT THIS TRUSTS, stated plainly, because you are about to pipe it into a shell:
#
#   1. TLS to hive-mcp.com, which redirects to the release asset on GitHub.
#   2. The release assets themselves. Every binary is checked against a signed
#      SHA256SUMS before it is made executable, so this is the part you do NOT
#      have to take on faith. See "How this verifies itself" below.
#   3. Nothing else. It never asks for root, never writes outside $BIN_DIR and
#      the skills directory, and never edits a shell rc file unless you pass
#      --setup.
#
# HOW THIS VERIFIES ITSELF
#
#   Tier 1  sha256 of every downloaded file against SHA256SUMS.  ALWAYS. Hard fail.
#   Tier 2  Ed25519 signature over SHA256SUMS, public key pinned below.
#           Needs an openssl that does raw Ed25519; probed with an RFC 8032
#           vector, so "cannot check" is never reported as "checked".
#   Tier 3  GitHub build provenance (which workflow, which commit) via `gh`.
#           Opportunistic; install `gh` to get it.
#
#   The tiers are reported at the end. A tier that could not run says so; it is
#   never silently counted as a pass.
#
# VERIFYING THE SCRIPT ITSELF, if you would rather not pipe an unread file:
#
#   curl -fsSL https://hive-mcp.com/install.sh -o install.sh
#   curl -fsSL https://hive-mcp.com/install.sh.sha256          # our cluster
#   sha256sum -c install.sh.sha256                             # or: shasum -a 256 -c
#   less install.sh && sh install.sh
#
#   The script comes from GitHub and the checksum from our own cluster, on
#   purpose: they are separate systems, so passing the check means BOTH would
#   have to be compromised, not either one.
#
# Options:
#   --setup            also run `hive setup` when the install succeeds
#   --emacs            with --setup, include the Emacs vessel steps
#   --bin-dir DIR      where to put the binaries (default ~/.local/bin)
#   --version VERSION  install a specific release instead of the latest
#   --no-skills        skip installing the setup skills
#   -h, --help         this
#
# Environment: HIVE_BIN_DIR, HIVE_SKILLS_DIR, HIVE_VERSION.

set -eu

REPO="hive-agi/hive-mcp-cli"

# Ed25519 public key that signs SHA256SUMS, as base64 of its SPKI DER.
# Published at https://hive-mcp.com/hive-signing-key.pub so you can compare this
# pinned copy against a different origin before trusting it.
# EMPTY until the key ceremony has run: see bin/gen-signing-key. While it is
# empty, tier 2 reports itself unavailable rather than passing.
HIVE_SIGNING_KEY=""

# RFC 8032 section 7.1 TEST 2. A published vector, so anyone can confirm that
# what the probe proves is "this openssl verifies Ed25519 correctly".
SELFTEST_KEY="MCowBQYDK2VwAyEAPUAXw+hDiVqStwqnTRt+vJyYLM8uxJaMwM1V8Sr0Zgw="
SELFTEST_MSG_HEX="72"
SELFTEST_SIG_HEX="92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00"

BIN_DIR="${HIVE_BIN_DIR:-$HOME/.local/bin}"
VERSION="${HIVE_VERSION:-}"
RUN_SETUP=0
EMACS=0
SKILLS=1

TIER_SUM="not reached"
TIER_SIG="not reached"
TIER_PROV="not reached"
TMP=""

# --- output ----------------------------------------------------------------

if [ -t 1 ]; then
  B="$(printf '\033[1m')"; DIM="$(printf '\033[2m')"; R="$(printf '\033[0m')"
  GREEN="$(printf '\033[32m')"; YELLOW="$(printf '\033[33m')"; RED="$(printf '\033[31m')"
else
  B=''; DIM=''; R=''; GREEN=''; YELLOW=''; RED=''
fi

say()  { printf '%s\n' "$*"; }
step() { printf '%s==>%s %s\n' "$B" "$R" "$*"; }
ok()   { printf '    %s%s%s %s\n' "$GREEN" "ok" "$R" "$*"; }
warn() { printf '    %s%s%s %s\n' "$YELLOW" "--" "$R" "$*"; }
die()  { printf '%serror:%s %s\n' "$RED" "$R" "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

cleanup() { [ -n "$TMP" ] && [ -d "$TMP" ] && rm -rf "$TMP"; }

usage() {
  cat <<'USAGE'
hive-mcp installer

  curl -fsSL https://hive-mcp.com/install.sh | sh

Options:
  --setup            also run `hive setup` when the install succeeds
  --emacs            with --setup, include the Emacs vessel steps
  --bin-dir DIR      where to put the binaries (default ~/.local/bin)
  --version VERSION  install a specific release instead of the latest
  --no-skills        skip installing the setup skills
  -h, --help         this

Every downloaded file is checked against a signed SHA256SUMS before it is made
executable. To read the script before running it, and to check it against a
second origin:

  curl -fsSL https://hive-mcp.com/install.sh -o install.sh
  curl -fsSL https://hive-mcp.com/install.sh.sha256
  sha256sum -c install.sh.sha256
  less install.sh && sh install.sh

Environment: HIVE_BIN_DIR, HIVE_SKILLS_DIR, HIVE_VERSION
USAGE
}

# --- platform --------------------------------------------------------------

detect_platform() {
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Linux)  OS=linux ;;
    Darwin) OS=darwin ;;
    *) die "unsupported OS: $os. Build from source: go install github.com/$REPO/cmd/hive@latest" ;;
  esac
  case "$arch" in
    x86_64|amd64) ARCH=amd64 ;;
    arm64|aarch64) ARCH=arm64 ;;
    *) die "unsupported architecture: $arch. Build from source: go install github.com/$REPO/cmd/hive@latest" ;;
  esac
}

# --- fetching --------------------------------------------------------------

fetch() {
  # fetch URL DEST. Quiet, and fails on a 4xx/5xx rather than saving the error page.
  if have curl; then
    curl -fsSL "$1" -o "$2"
  elif have wget; then
    wget -qO "$2" "$1"
  else
    return 1
  fi
}

latest_version() {
  # The release tag, minus its leading v. Read from the redirect Location header
  # rather than the API, which rate-limits unauthenticated callers to a number an
  # install script can genuinely hit on a shared address.
  have curl || return 1
  curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/$REPO/releases/latest" 2>/dev/null \
    | sed -n 's|.*/tag/v\{0,1\}\(.*\)$|\1|p'
}

# --- verification ----------------------------------------------------------

sha256_of() {
  # One hex digest for file $1, on whichever of the three tools exists.
  if have sha256sum; then
    sha256sum "$1" | cut -d' ' -f1
  elif have shasum; then
    shasum -a 256 "$1" | cut -d' ' -f1
  elif have openssl; then
    openssl dgst -sha256 "$1" | sed 's/.*= *//'
  else
    return 1
  fi
}

openssl_verifies_ed25519() {
  # Probe with a PUBLISHED vector, so a failure here means "this openssl cannot
  # do the check" and never gets confused with "the signature is bad". macOS
  # ships LibreSSL as `openssl`, which cannot, and that must degrade loudly
  # rather than look like an attack, or look like a pass.
  have openssl || return 1
  have xxd || return 1
  printf '%s' "$SELFTEST_KEY" | base64 -d > "$TMP/selftest.der" 2>/dev/null || return 1
  printf '%s' "$SELFTEST_MSG_HEX" | xxd -r -p > "$TMP/selftest.msg" 2>/dev/null || return 1
  printf '%s' "$SELFTEST_SIG_HEX" | xxd -r -p > "$TMP/selftest.sig" 2>/dev/null || return 1
  openssl pkeyutl -verify -pubin -inkey "$TMP/selftest.der" -keyform DER \
    -rawin -in "$TMP/selftest.msg" -sigfile "$TMP/selftest.sig" >/dev/null 2>&1
}

verify_signature() {
  # Tier 2. Sets TIER_SIG. Returns non-zero ONLY when a check that could run
  # actually failed, which is the case that must stop the install.
  if [ -z "$HIVE_SIGNING_KEY" ]; then
    TIER_SIG="unavailable: no key pinned in this build of the installer"
    return 0
  fi
  if [ ! -f "$TMP/SHA256SUMS.sig" ]; then
    TIER_SIG="FAILED: release carries SHA256SUMS but no signature"
    return 1
  fi
  if ! openssl_verifies_ed25519; then
    TIER_SIG="unavailable: no openssl that verifies Ed25519 (macOS: brew install openssl@3)"
    return 0
  fi
  printf '%s' "$HIVE_SIGNING_KEY" | base64 -d > "$TMP/signing.der" 2>/dev/null \
    || { TIER_SIG="FAILED: pinned key is not valid base64"; return 1; }
  if openssl pkeyutl -verify -pubin -inkey "$TMP/signing.der" -keyform DER \
       -rawin -in "$TMP/SHA256SUMS" -sigfile "$TMP/SHA256SUMS.sig" >/dev/null 2>&1; then
    TIER_SIG="verified: Ed25519 over SHA256SUMS"
    return 0
  fi
  TIER_SIG="FAILED: SHA256SUMS is not signed by the pinned key"
  return 1
}

verify_checksum() {
  # Tier 1, for one file. Always runs, always decides.
  name="$1"
  want="$(sed -n "s/^\([0-9a-f]\{64\}\)[ *]*$name\$/\1/p" "$TMP/SHA256SUMS" | head -1)"
  [ -n "$want" ] || die "SHA256SUMS names no entry for $name. Refusing to install an unlisted file."
  got="$(sha256_of "$TMP/$name")" \
    || die "no sha256 tool found (sha256sum, shasum or openssl). Refusing to install unverified binaries."
  [ "$want" = "$got" ] || die "checksum mismatch for $name.
    expected $want
    got      $got
    This is what a tampered or truncated download looks like. Nothing was installed."
}

verify_provenance() {
  # Tier 3. Opportunistic: names the workflow and commit that built the binary,
  # which is the one check a stolen signing key would not survive.
  if ! have gh; then
    TIER_PROV="unavailable: gh not installed"
    return 0
  fi
  if gh attestation verify "$TMP/hive" --repo "$REPO" >/dev/null 2>&1; then
    TIER_PROV="verified: built by $REPO's release workflow"
  else
    TIER_PROV="unavailable: no attestation published for this release yet"
  fi
  return 0
}

download_and_verify() {
  base="https://github.com/$REPO/releases/download/v$VERSION"
  fetch "$base/SHA256SUMS" "$TMP/SHA256SUMS" \
    || die "this release publishes no SHA256SUMS, so nothing can be verified.
    Refusing to install. Build from source instead:
      go install github.com/$REPO/cmd/hive@latest"
  fetch "$base/SHA256SUMS.sig" "$TMP/SHA256SUMS.sig" 2>/dev/null || true

  for tool in hive hive-setup-mcp; do
    fetch "$base/$tool-$OS-$ARCH" "$TMP/$tool" \
      || die "no release binary for $OS/$ARCH in v$VERSION"
  done

  verify_signature || die "$TIER_SIG
    SHA256SUMS did not verify against the key pinned in this installer.
    Nothing was installed. Do not retry: report this."

  for tool in hive hive-setup-mcp; do
    verify_checksum "$tool-$OS-$ARCH"
    mv "$TMP/$tool" "$TMP/$tool.verified" && mv "$TMP/$tool.verified" "$TMP/$tool"
  done
  TIER_SUM="verified: sha256 of every file matched SHA256SUMS"

  verify_provenance
}

install_verified() {
  for tool in hive hive-setup-mcp; do
    chmod +x "$TMP/$tool"
    mv "$TMP/$tool" "$BIN_DIR/$tool"
  done
}

install_with_go() {
  have go || return 1
  warn "no verified release available; building from source with Go instead"
  GOBIN="$BIN_DIR" go install "github.com/$REPO/cmd/hive@latest" || return 1
  GOBIN="$BIN_DIR" go install "github.com/$REPO/cmd/hive-setup-mcp@latest" || return 1
  TIER_SUM="not applicable: built from source by go install"
  TIER_SIG="not applicable: built from source by go install"
  TIER_PROV="not applicable: built from source by go install"
  return 0
}

report_trust() {
  say ""
  say "${B}What was verified${R}"
  printf '  %-12s %s\n' "checksums" "$TIER_SUM"
  printf '  %-12s %s\n' "signature" "$TIER_SIG"
  printf '  %-12s %s\n' "provenance" "$TIER_PROV"
}

# --- main ------------------------------------------------------------------

main() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --setup) RUN_SETUP=1 ;;
      --emacs) EMACS=1 ;;
      --no-skills) SKILLS=0 ;;
      --bin-dir) shift; BIN_DIR="${1:?--bin-dir needs a path}" ;;
      --version) shift; VERSION="${1:?--version needs a version}" ;;
      -h|--help) usage; exit 0 ;;
      *) die "unknown option: $1" ;;
    esac
    shift
  done

  trap cleanup EXIT INT TERM
  TMP="$(mktemp -d)"

  say ""
  say "${B}hive-mcp${R}: persistent memory, a knowledge graph and a swarm for Claude Code"
  say ""

  detect_platform
  mkdir -p "$BIN_DIR"

  step "Installing the hive CLI ($OS/$ARCH)"
  [ -n "$VERSION" ] || VERSION="$(latest_version || true)"
  if [ -n "$VERSION" ]; then
    download_and_verify
    install_verified
    ok "hive $VERSION -> $BIN_DIR"
  elif install_with_go; then
    ok "built from source -> $BIN_DIR"
  else
    die "could not install the CLI.
    No published release to verify and no Go toolchain to build one.
    Install Go 1.21+ (https://go.dev/dl/) and re-run, or clone and build:
      git clone https://github.com/$REPO && cd hive-mcp-cli && go build ./cmd/hive"
  fi

  PATH="$BIN_DIR:$PATH"
  export PATH

  if [ "$SKILLS" -eq 1 ]; then
    step "Installing the setup skills"
    "$BIN_DIR/hive" guide --install
  fi

  if have claude; then
    step "Registering the setup helper with Claude Code"
    if claude mcp list 2>/dev/null | grep -q '^hive-setup'; then
      warn "hive-setup is already registered"
    elif claude mcp add hive-setup --scope user -- "$BIN_DIR/hive-setup-mcp" >/dev/null 2>&1; then
      ok "hive-setup registered"
    else
      warn "could not register hive-setup; run: claude mcp add hive-setup --scope user -- $BIN_DIR/hive-setup-mcp"
    fi
  else
    warn "Claude Code not found on PATH. Install it from https://claude.ai/download"
  fi

  case ":$PATH:" in
    *":$BIN_DIR:"*) ;;
    *) warn "$BIN_DIR is not on your PATH. Add it to your shell rc file." ;;
  esac

  if [ "$RUN_SETUP" -eq 1 ]; then
    step "Running hive setup"
    if [ "$EMACS" -eq 1 ]; then
      "$BIN_DIR/hive" setup --emacs
    else
      "$BIN_DIR/hive" setup
    fi
    "$BIN_DIR/hive" doctor || true
  fi

  report_trust

  say ""
  say "${B}Done.${R} Start Claude Code in a project and say one of these:"
  say ""
  say "  ${B}\"help me set up the hive-mcp harness locally, I have a key\"${R}"
  say "  ${B}\"help me set up a FOSS build of the hive-mcp harness\"${R}"
  say ""
  say "${DIM}It reads the skills just installed and drives the rest.${R}"
  say "${DIM}Prefer to do it yourself: ${R}hive detect${DIM}, then ${R}hive setup${DIM}, then ${R}hive doctor${DIM}.${R}"
  say "${DIM}Docs: https://docs.hive-mcp.com   Addons: https://store.hive-mcp.com${R}"
  say ""
}

# Called on the LAST line on purpose. `curl | sh` executes what it has received
# so far, so a connection that drops mid-transfer would otherwise run half a
# script. Nothing above this line does anything on its own, and a truncated copy
# never reaches this call.
main "$@"
