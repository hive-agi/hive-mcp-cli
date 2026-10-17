#!/bin/sh
# Exercise tier-three verdicts without network or a release download.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT HUP INT TERM
# The installer invokes main only on the final line; source its definitions.
sed '$d' "$ROOT/install.sh" > "$WORK/functions.sh"
. "$WORK/functions.sh"
TMP="$WORK"
printf 'binary bytes' > "$TMP/hive"
REPO='hive-agi/hive-mcp-cli'
have() { [ "$1" = gh ]; }
fixture_digest='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
sha256_of() { printf '%s\n' "$fixture_digest"; }
VERSION=0.3.2 OS=linux ARCH=amd64
release_digest="sha256:$fixture_digest"
gh() {
  case "$MODE:$1:$2" in
    old:attestation:--help) return 1 ;;
    *:attestation:--help) return 0 ;;
    anon:auth:status) return 1 ;;
    *:auth:status) return 0 ;;
    *:api:repos/$REPO/releases/tags/v$VERSION) printf '%s\n' "$release_digest" ;;
    api-error:api:*) return 1 ;;
    invalid:api:repos/$REPO/attestations/$release_digest) printf 'invalid\n' ;;
    *:api:repos/$REPO/attestations/$release_digest) printf '%s\n' "$COUNT" ;;
    bad:attestation:verify) return 1 ;;
    good:attestation:verify) return 0 ;;
    *) printf 'unexpected gh call: %s\n' "$*" >&2; return 1 ;;
  esac
}
MODE=old
verify_provenance
[ "$TIER_PROV" = 'unavailable: gh too old (needs >= 2.49 for attestation)' ]
MODE=anon
verify_provenance
[ "$TIER_PROV" = 'unavailable: gh not logged in (run gh auth login to check provenance)' ]
MODE=none COUNT=0
verify_provenance
[ "$TIER_PROV" = 'unavailable: no attestation published for this release yet' ]
MODE=bad COUNT=1
if verify_provenance; then printf 'mismatch passed\n' >&2; exit 1; fi
[ "$TIER_PROV" = 'FAILED: published attestation did not verify for this binary' ]
MODE=good COUNT=1
verify_provenance
[ "$TIER_PROV" = "verified: built by $REPO's release workflow" ]
MODE=api-error COUNT=1
if verify_provenance; then printf 'API failure passed\n' >&2; exit 1; fi
[ "$TIER_PROV" = 'FAILED: cannot query published attestations' ]
MODE=invalid
if verify_provenance; then printf 'malformed attestation response passed\n' >&2; exit 1; fi
[ "$TIER_PROV" = 'FAILED: invalid attestation API response' ]
# A substituted binary's digest must not be used to declare attestation absent.
# Exercise download, checksums, provenance and the installation boundary together.
mkdir -p "$WORK/release" "$WORK/bin" "$WORK/download"
printf 'substituted binary' > "$WORK/release/hive-linux-amd64"
printf 'setup binary' > "$WORK/release/hive-setup-mcp-linux-amd64"
(
  cd "$WORK/release"
  sha256sum hive-linux-amd64 hive-setup-mcp-linux-amd64 > SHA256SUMS
)
# The published digest belongs to an attested release artifact (COUNT=1),
# while the substituted download and rewritten SHA256SUMS agree with each other.
TMP="$WORK/download" BIN_DIR="$WORK/bin" MODE=none COUNT=1
sha256_of() { sha256sum "$1" | cut -d' ' -f1; }
fetch() { cp "$WORK/release/$(basename "$1")" "$2"; }
verify_signature() { TIER_SIG='unavailable: test fixture'; }
if (download_and_verify && install_verified) > "$WORK/output" 2>&1; then
  printf 'substituted binary installed\n' >&2; exit 1
fi
if ! grep -q 'FAILED: binary differs from published release asset digest' "$WORK/output"; then
  cat "$WORK/output" >&2; exit 1
fi
[ ! -e "$BIN_DIR/hive" ] && [ ! -e "$BIN_DIR/hive-setup-mcp" ]
printf '8 provenance verdicts passed (including substituted binary installation refusal)\n'
