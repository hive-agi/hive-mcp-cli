#!/usr/bin/env bash
# fake-browser.sh URL: plays the user at a browser, for the auth e2e test.
# `hive login` launches it through $BROWSER exactly as it would launch Firefox.
# It follows the real Keycloak pages: fills the login form, accepts the device
# grant's consent screen, and follows the final redirect, which for the
# loopback flow lands on the CLI's own 127.0.0.1 listener.
set -u
USER_NAME="${E2E_USER:-customer@example.com}"
USER_PASS="${E2E_PASS:-correct horse}"
LOG="${E2E_BROWSER_LOG:-/dev/stderr}"
jar="$(mktemp)"
page="$(mktemp)"
trap 'rm -f "$jar" "$page"' EXIT

ORIGIN="$(printf '%s' "$1" | sed -E 's#^(https?://[^/]+).*#\1#')"
form_action() {
  local a
  a="$(grep -o '<form[^>]*action="[^"]*"' "$page" | head -1 | sed 's/.*action="//; s/"$//; s/&amp;/\&/g')"
  case "$a" in /*) a="$ORIGIN$a" ;; esac  # the device consent form posts to a relative path
  printf '%s' "$a"
}

curl -sSL -c "$jar" -b "$jar" -o "$page" "$1" 2>>"$LOG"
for step in 1 2 3 4 5; do
  if grep -q 'Signed in to hive\|Device Login Successful\|successfully logged\|device-login-success' "$page"; then
    echo "browser: done after $step page(s)" >>"$LOG"; exit 0
  fi
  action="$(form_action)"
  [ -n "$action" ] || { echo "browser: no form on page:" >>"$LOG"; sed 's/<[^>]*>//g' "$page" | tr -s ' \n' | head -c 600 >>"$LOG"; exit 1; }
  if grep -q 'name="password"' "$page"; then
    echo "browser: signing in" >>"$LOG"
    curl -sSL -c "$jar" -b "$jar" -o "$page" --data-urlencode "username=$USER_NAME" \
      --data-urlencode "password=$USER_PASS" --data-urlencode "credentialId=" "$action" 2>>"$LOG"
  elif grep -q 'name="accept"' "$page"; then
    echo "browser: granting access" >>"$LOG"
    curl -sSL -c "$jar" -b "$jar" -o "$page" --data-urlencode "accept=Yes" "$action" 2>>"$LOG"
  elif grep -q 'name="device_user_code"' "$page"; then
    echo "browser: entering code ${E2E_USER_CODE:-?}" >>"$LOG"
    curl -sSL -c "$jar" -b "$jar" -o "$page" --data-urlencode "device_user_code=${E2E_USER_CODE:-}" "$action" 2>>"$LOG"
  else
    echo "browser: unknown form at $action" >>"$LOG"; sed 's/<[^>]*>//g' "$page" | tr -s ' \n' | head -c 600 >>"$LOG"; exit 1
  fi
done
echo "browser: gave up" >>"$LOG"; exit 1
