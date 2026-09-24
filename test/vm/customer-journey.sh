#!/usr/bin/env bash
# customer-journey.sh: the whole first-run of a paying customer, on a fresh VM.
#
#   test/vm/customer-journey.sh            released bits: curl hive-mcp.com/install.sh | sh
#   test/vm/customer-journey.sh --dev      this checkout's install.sh and hive binary, and
#                                          HIVE_MCP_LAUNCHER (a bin/hive-mcp-foss) if set
#
# Starts from the `clean` snapshot of hive-vm-customer (made on first run), then:
#   10 install Claude Code        20 install.sh        30 hive setup
#   40 new shell, other project: claude mcp list says hive is Connected
#   50 a local.deps.edn overlay (where licensed coordinates go) reaches the classpath
#   60 hive store login refuses a bad token; hive addon add writes the overlay
#      (HIVE_JOURNEY_TOKEN, forwarded to the guest by hand, proves a real token)
# Each stage's output lands in $HIVE_VM_CACHE/journey/<stage>.log and the run ends
# with one PASS/FAIL line per stage. The VM is left running at the end, for poking.

set -uo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
VM="$HERE/hive-vm"
NAME=customer
LOGS="${HIVE_VM_CACHE:-$HOME/.cache/hive-vm}/journey"
DEV=0
[ "${1:-}" = "--dev" ] && DEV=1
mkdir -p "$LOGS"

declare -a RESULTS=()
stage() {
  # stage LABEL SCRIPT: run SCRIPT in the guest, record PASS/FAIL, keep going.
  local label="$1" script="$2" log="$LOGS/$1.log" t0=$SECONDS
  printf '==> %s\n' "$label" >&2
  if "$VM" run "$NAME" "$script" >"$log" 2>&1; then
    RESULTS+=("PASS  $label  ($((SECONDS - t0))s)")
  else
    RESULTS+=("FAIL  $label  ($((SECONDS - t0))s)  see $log")
    tail -15 "$log" >&2
  fi
}

if "$VM" list | grep -q "hive-vm-$NAME"; then
  "$VM" back "$NAME" clean || exit 1
else
  "$VM" up "$NAME" && "$VM" snap "$NAME" clean || exit 1
fi

stage 10-claude "$HERE/guest/10-claude.sh"

if [ "$DEV" = 1 ]; then
  bin="$(mktemp -d)/hive"
  (cd "$ROOT" && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o "$bin" ./cmd/hive) || exit 1
  stage 20-install "$ROOT/install.sh"
  "$VM" push "$NAME" "$bin" .local/bin/hive
else
  printf 'curl -fsSL https://hive-mcp.com/install.sh | sh\n' >"$LOGS/20-install.sh"
  stage 20-install "$LOGS/20-install.sh"
fi

stage 30-setup "$HERE/guest/30-setup.sh"

if [ "$DEV" = 1 ] && [ -n "${HIVE_MCP_LAUNCHER:-}" ]; then
  # The launcher lives in the checkout setup cloned. Setup never boots it, so
  # swapping it in here still precedes its first run.
  "$VM" push "$NAME" "$HIVE_MCP_LAUNCHER" hive-mcp/bin/hive-mcp-foss
fi

stage 40-connect "$HERE/guest/40-connect.sh"
stage 50-overlay "$HERE/guest/50-overlay.sh"
stage 60-licensed-cli "$HERE/guest/60-licensed-cli.sh"

printf '\n'
printf '%s\n' "${RESULTS[@]}"
! printf '%s\n' "${RESULTS[@]}" | grep -q '^FAIL'
