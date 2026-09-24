#!/usr/bin/env bash
# e2e.sh: `hive login` against a REAL Keycloak, end to end.
#
# Starts Keycloak 26.6.1 (the version production runs) with a realm whose
# hive-cli client matches k8s-agi/terraform/keycloak-hive-realm/clients-cli.tf,
# and a fake store that enforces what the real one does (aud=hive-store,
# typ=Bearer, a token the issuer accepts). Then drives the real `hive` binary
# through: browser login (loopback + PKCE), auth status, auth token, logout,
# device-code login, logout. The "browser" is fake-browser.sh, launched through
# $BROWSER, filling the real Keycloak pages.
#
# Needs docker. Leaves nothing behind: temp HOME, temp config, container removed.
set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
KC_PORT="${KC_PORT:-18180}"
STORE_PORT="${STORE_PORT:-18181}"
NAME="hive-auth-e2e-kc"
WORK="$(mktemp -d)"
export E2E_BROWSER_LOG="$WORK/browser.log"
pass=0; fail=0
ok()  { printf 'PASS  %s\n' "$*"; pass=$((pass+1)); }
bad() { printf 'FAIL  %s\n' "$*"; fail=$((fail+1)); }

cleanup() {
  docker rm -f "$NAME" >/dev/null 2>&1
  [ -n "${STORE_PID:-}" ] && kill "$STORE_PID" 2>/dev/null
  rm -rf "$WORK"
}
trap cleanup EXIT

echo "==> Keycloak 26.6.1 on :$KC_PORT"
docker rm -f "$NAME" >/dev/null 2>&1
docker run -d --name "$NAME" -p "127.0.0.1:$KC_PORT:8080" \
  -e KC_BOOTSTRAP_ADMIN_USERNAME=admin -e KC_BOOTSTRAP_ADMIN_PASSWORD=admin \
  -v "$HERE/realm-hive-test.json:/opt/keycloak/data/import/realm.json:ro" \
  quay.io/keycloak/keycloak:26.6.1 start-dev --import-realm >/dev/null || exit 1
ISSUER="http://127.0.0.1:$KC_PORT/realms/hive"
for i in $(seq 1 90); do curl -sf "$ISSUER/.well-known/openid-configuration" >/dev/null && break; sleep 2; done
curl -sf "$ISSUER/.well-known/openid-configuration" >/dev/null || { echo "keycloak did not come up"; docker logs "$NAME" | tail -20; exit 1; }

echo "==> build"
(cd "$ROOT" && go build -o "$WORK/hive" ./cmd/hive && go build -o "$WORK/fakestore" ./test/auth/fakestore) || exit 1
"$WORK/fakestore" -addr "127.0.0.1:$STORE_PORT" -userinfo "$ISSUER/protocol/openid-connect/userinfo" 2>"$WORK/store.log" &
STORE_PID=$!
sleep 1

# A customer's machine, in a box: its own HOME (so ~/.m2 is the test's), its
# own config dir, the file credential store (no desktop keyring in CI).
export HOME="$WORK/home"; mkdir -p "$HOME"
export HIVE_CONFIG_DIR="$WORK/config" HIVE_CREDENTIAL_STORE=file
export HIVE_AUTH_ISSUER="$ISSUER" HIVE_STORE_URL="http://127.0.0.1:$STORE_PORT"
export BROWSER="$HERE/fake-browser.sh"
unset SSH_CONNECTION SSH_TTY HIVE_STORE_TOKEN
H="$WORK/hive"
live() { curl -s "$HIVE_STORE_URL/api/_live" | sed 's/[^0-9]//g'; }

echo "==> hive login (browser, loopback + PKCE)"
if timeout 120 "$H" login >"$WORK/login1.out" 2>&1; then ok "browser login"; else bad "browser login"; cat "$WORK/login1.out" "$E2E_BROWSER_LOG"; fi
grep -q 'Signed in as customer@example.com' "$WORK/login1.out" && ok "store saw the signed-in email" || bad "no email in output"
grep -q '<username>hv_live_' "$HOME/.m2/settings.xml" 2>/dev/null && ok "artifact token in settings.xml" || bad "settings.xml not written"
[ "$(stat -c %a "$HOME/.m2/settings.xml" 2>/dev/null)" = 600 ] && ok "settings.xml is 0600" || bad "settings.xml mode"
[ "$(stat -c %a "$HIVE_CONFIG_DIR/credentials.json" 2>/dev/null)" = 600 ] && ok "credentials.json is 0600" || bad "credentials.json mode"
grep -q hv_live_ "$HIVE_CONFIG_DIR/session.json" && bad "a secret leaked into session.json" || ok "session.json holds no secret"
[ "$(live)" = 1 ] && ok "one live artifact token" || bad "live tokens: $(live)"

"$H" auth status >"$WORK/status.out" 2>&1 && grep -q 'Signed in as customer@example.com (active)' "$WORK/status.out" \
  && ok "auth status" || { bad "auth status"; cat "$WORK/status.out"; }
tok="$("$H" auth token 2>/dev/null)"
curl -sf -H "Authorization: Bearer $tok" "$HIVE_STORE_URL/api/me" >/dev/null && ok "auth token works against the store" || bad "auth token"

echo "==> refresh: an expired access token is renewed from the stored refresh token"
python3 - "$HIVE_CONFIG_DIR/credentials.json" <<'PY'
import json,sys
p=sys.argv[1]; d=json.load(open(p)); t=json.loads(d["oidc-token"])
t["expires_at"]="2000-01-01T00:00:00Z"; d["oidc-token"]=json.dumps(t); json.dump(d,open(p,"w"))
PY
tok2="$("$H" auth token 2>/dev/null)"
[ -n "$tok2" ] && [ "$tok2" != "$tok" ] && ok "refreshed access token" || bad "refresh"

echo "==> hive login again replaces the first machine token"
timeout 120 "$H" login >"$WORK/login2.out" 2>&1 && [ "$(live)" = 1 ] && ok "re-login leaves one live token" || { bad "re-login (live=$(live))"; cat "$WORK/login2.out"; }

echo "==> hive logout"
"$H" logout >"$WORK/logout.out" 2>&1 && ok "logout" || { bad "logout"; cat "$WORK/logout.out"; }
[ "$(live)" = 0 ] && ok "artifact token revoked at the store" || bad "still live: $(live)"
grep -q hive-store "$HOME/.m2/settings.xml" 2>/dev/null && bad "settings.xml still holds hive-store" || ok "settings.xml cleaned"
[ -e "$HIVE_CONFIG_DIR/credentials.json" ] && bad "credentials left behind" || ok "credentials removed"

echo "==> hive login --device (one-time code)"
# The fake browser is handed the complete verification URI, as a user would
# click it; on a real headless box the user types the code on another device.
timeout 120 "$H" login --device >"$WORK/device.out" 2>&1 && ok "device login" || { bad "device login"; cat "$WORK/device.out" "$E2E_BROWSER_LOG"; }
grep -qE 'one-time code: +[A-Z0-9]{4}-[A-Z0-9]{4}' "$WORK/device.out" && ok "user code shown" || bad "no user code shown"
"$H" logout >/dev/null 2>&1

echo "==> an account without offline_access still signs in (session-length)"
E2E_USER=no-offline@example.com timeout 120 "$H" login >"$WORK/nooffline.out" 2>&1 \
  && grep -q 'session-length' "$WORK/nooffline.out" && grep -q 'Signed in as no-offline@example.com' "$WORK/nooffline.out" \
  && ok "offline_access fallback" || { bad "offline_access fallback"; cat "$WORK/nooffline.out"; }
"$H" logout >/dev/null 2>&1

echo
echo "$pass passed, $fail failed"
[ "$fail" = 0 ]
