#!/bin/sh
# Proves install.sh's verification gate in every state that matters.
#
# A gate is only worth having if it FAILS when it should. Each case below is
# checked for the verdict AND for the reason, because "could not check" and
# "check failed" must never collapse into each other.
# shellcheck disable=SC1091,SC2034  # lib.sh is generated at run time from install.sh
set -eu

here="$(cd "$(dirname "$0")" && pwd)"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Load the installer's functions without running it.
sed 's/^main "\$@"$//' "$here/../install.sh" > "$work/lib.sh"

pass=0; fail=0
check() { # check LABEL EXPECTED ACTUAL
  if [ "$2" = "$3" ]; then pass=$((pass+1)); printf '  ok    %s\n' "$1"
  else fail=$((fail+1)); printf '  FAIL  %s\n        expected: %s\n        got:      %s\n' "$1" "$2" "$3"; fi
}
check_has() { # check_has LABEL NEEDLE HAYSTACK
  case "$3" in
    *"$2"*) pass=$((pass+1)); printf '  ok    %s\n' "$1" ;;
    *) fail=$((fail+1)); printf '  FAIL  %s\n        wanted to contain: %s\n        got: %s\n' "$1" "$2" "$3" ;;
  esac
}

# A throwaway signing key, made here and thrown away with $work.
openssl genpkey -algorithm ed25519 -out "$work/priv.pem" 2>/dev/null
openssl pkey -in "$work/priv.pem" -pubout -outform DER -out "$work/pub.der" 2>/dev/null
PUB_B64="$(base64 -w0 < "$work/pub.der" 2>/dev/null || base64 < "$work/pub.der" | tr -d '\n')"

new_release() { # new_release DIR -> a fake release with two binaries and signed sums
  d="$1"; mkdir -p "$d"
  printf 'fake hive binary\n' > "$d/hive-linux-amd64"
  printf 'fake setup binary\n' > "$d/hive-setup-mcp-linux-amd64"
  ( cd "$d" && sha256sum hive-linux-amd64 hive-setup-mcp-linux-amd64 > SHA256SUMS )
  openssl pkeyutl -sign -inkey "$work/priv.pem" -rawin \
    -in "$d/SHA256SUMS" -out "$d/SHA256SUMS.sig" 2>/dev/null
}

run_case() { # run_case NAME TMPDIR KEY  -> echoes "<rc>|<TIER_SIG>"
  ( . "$work/lib.sh"
    set +e
    TMP="$2"; HIVE_SIGNING_KEY="$3"
    verify_signature; rc=$?
    printf '%s|%s' "$rc" "$TIER_SIG" )
}

echo "== signature gate"

new_release "$work/good"
out="$(run_case good "$work/good" "$PUB_B64")"
check "a signed SHA256SUMS verifies" "0" "${out%%|*}"
check_has "  and says so" "verified: Ed25519" "${out#*|}"

new_release "$work/tampered"
printf '0000000000000000000000000000000000000000000000000000000000000000  hive-linux-amd64\n' \
  > "$work/tampered/SHA256SUMS"
out="$(run_case tampered "$work/tampered" "$PUB_B64")"
check "a tampered SHA256SUMS is REFUSED" "1" "${out%%|*}"
check_has "  and is reported as a failure, not a skip" "FAILED" "${out#*|}"

new_release "$work/wrongkey"
openssl genpkey -algorithm ed25519 -out "$work/other.pem" 2>/dev/null
openssl pkey -in "$work/other.pem" -pubout -outform DER -out "$work/other.der" 2>/dev/null
OTHER_B64="$(base64 -w0 < "$work/other.der" 2>/dev/null || base64 < "$work/other.der" | tr -d '\n')"
out="$(run_case wrongkey "$work/wrongkey" "$OTHER_B64")"
check "a signature by the WRONG key is REFUSED" "1" "${out%%|*}"

new_release "$work/nosig"; rm -f "$work/nosig/SHA256SUMS.sig"
out="$(run_case nosig "$work/nosig" "$PUB_B64")"
check "a missing signature, with a key pinned, is REFUSED" "1" "${out%%|*}"

new_release "$work/nokey"
out="$(run_case nokey "$work/nokey" "")"
check "no key pinned does not block the install" "0" "${out%%|*}"
check_has "  but is reported as UNAVAILABLE, never as verified" "unavailable" "${out#*|}"

echo "== capability probe"
out="$( . "$work/lib.sh"; set +e; TMP="$work/good"; openssl_verifies_ed25519; echo $? )"
check "the RFC 8032 vector verifies on this openssl" "0" "$out"

echo "== checksum gate"
new_release "$work/sums"
status_of() { # status_of TMPDIR NAME
  if ( . "$work/lib.sh"; TMP="$1"; verify_checksum "$2" ) >/dev/null 2>&1
  then echo 0; else echo 1; fi
}
check "a matching checksum passes" "0" "$(status_of "$work/sums" hive-linux-amd64)"

printf 'swapped for something else\n' > "$work/sums/hive-linux-amd64"
check "a swapped binary is REFUSED" "1" "$(status_of "$work/sums" hive-linux-amd64)"

new_release "$work/unlisted"
printf 'not in the sums file\n' > "$work/unlisted/hive-rogue-amd64"
check "a file absent from SHA256SUMS is REFUSED" "1" "$(status_of "$work/unlisted" hive-rogue-amd64)"

echo
echo "$pass passed, $fail failed"
[ "$fail" -eq 0 ]
