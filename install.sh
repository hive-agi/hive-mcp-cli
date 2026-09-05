#!/bin/sh
# hive-mcp installer.
#
#   curl -fsSL https://hive-mcp.com/install.sh | sh
#
# Installs the `hive` CLI and the setup skills, then tells you the sentence to
# say to Claude Code. It deliberately stops there by default: the machine setup
# proper (clone, prerequisites, Docker services, MCP registration) is what
# `hive setup` does, and running that unattended out of a pipe is not a thing to
# do to somebody's machine without asking. Pass --setup if you want it anyway.
#
#   --setup            also run `hive setup` when the install succeeds
#   --emacs            with --setup, include the Emacs vessel steps
#   --bin-dir DIR      where to put the binaries (default ~/.local/bin)
#   --version VERSION  install a specific release instead of the latest
#   --no-skills        skip installing the setup skills
#
# Environment: HIVE_BIN_DIR, HIVE_SKILLS_DIR, HIVE_VERSION.

set -eu

REPO="hive-agi/hive-mcp-cli"
BIN_DIR="${HIVE_BIN_DIR:-$HOME/.local/bin}"
VERSION="${HIVE_VERSION:-}"
RUN_SETUP=0
EMACS=0
SKILLS=1

# --- output ----------------------------------------------------------------
# Colour only when stdout is a terminal. Piped into `sh`, stdout usually is;
# redirected to a file it is not, and escape codes in a log help nobody.
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

# Piped into `sh`, $0 is not a file, so usage cannot be read back out of this
# script the way a downloaded copy could.
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

Environment: HIVE_BIN_DIR, HIVE_SKILLS_DIR, HIVE_VERSION
USAGE
}

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
  # The release tag, minus its leading v. Read from the redirect Location
  # header rather than the API, which rate-limits unauthenticated callers to
  # a number an install script can genuinely hit on a shared address.
  have curl || return 1
  curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/$REPO/releases/latest" 2>/dev/null \
    | sed -n 's|.*/tag/v\{0,1\}\(.*\)$|\1|p'
}

install_release() {
  [ -n "$VERSION" ] || VERSION="$(latest_version || true)"
  [ -n "$VERSION" ] || return 1
  base="https://github.com/$REPO/releases/download/v$VERSION"
  tmp="$(mktemp -d)"
  for tool in hive hive-setup-mcp; do
    fetch "$base/$tool-$OS-$ARCH" "$tmp/$tool" || { rm -rf "$tmp"; return 1; }
    chmod +x "$tmp/$tool"
    mv "$tmp/$tool" "$BIN_DIR/$tool"
  done
  rm -rf "$tmp"
  ok "hive $VERSION -> $BIN_DIR"
}

install_with_go() {
  have go || return 1
  GOBIN="$BIN_DIR" go install "github.com/$REPO/cmd/hive@latest" || return 1
  GOBIN="$BIN_DIR" go install "github.com/$REPO/cmd/hive-setup-mcp@latest" || return 1
  ok "built from source with Go -> $BIN_DIR"
}

# --- run -------------------------------------------------------------------

say ""
say "${B}hive-mcp${R}: persistent memory, a knowledge graph and a swarm for Claude Code"
say ""

detect_platform
mkdir -p "$BIN_DIR"

step "Installing the hive CLI ($OS/$ARCH)"
if install_release; then
  :
elif install_with_go; then
  :
else
  die "could not install the CLI.
    No release binary for $OS/$ARCH and no Go toolchain to build one.
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
