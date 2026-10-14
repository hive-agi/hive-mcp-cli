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
sha256_of() { printf 'abc123\n'; }
gh() {
  case "$MODE:$1:$2" in
    old:attestation:--help) return 1 ;;
    none:attestation:--help|bad:attestation:--help|good:attestation:--help|api-error:attestation:--help) return 0 ;;
    none:api:*|bad:api:*|good:api:*) printf '%s\n' "$COUNT" ;;
    api-error:api:*) return 1 ;;
    bad:attestation:verify) return 1 ;;
    good:attestation:verify) return 0 ;;
    *) printf 'unexpected gh call: %s\n' "$*" >&2; return 1 ;;
  esac
}
MODE=old
verify_provenance
[ "$TIER_PROV" = 'unavailable: gh too old (needs >= 2.49 for attestation)' ]
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
printf '5 provenance verdicts passed\n'
