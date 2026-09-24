# Stage: the licensed-path commands behave on a machine with no subscription.
# A real token needs a paid account, so this proves the refusals and the file
# handling; HIVE_JOURNEY_TOKEN (optional) proves the happy path.
export PATH="$HOME/.local/bin:$PATH"
fail=0

echo "--- a bogus token is rejected and nothing is written"
echo "hv_live_journey_bogus_token" | hive store login; rc=$?
[ "$rc" -ne 0 ] || { echo "bogus token was accepted"; fail=1; }
[ -f ~/.m2/settings.xml ] && grep -q hv_live_journey_bogus ~/.m2/settings.xml && { echo "bogus token was written"; fail=1; }

echo "--- --check reports, changes nothing"
hive store login --check || fail=1

echo "--- addon add writes the overlay, and recognises it the second time"
rm -f ~/hive-mcp/local.deps.edn
hive addon add hive-carto || fail=1
grep -q 'io.github.hive-agi/hive-carto ' ~/hive-mcp/local.deps.edn || { echo "overlay missing the coordinate"; fail=1; }
hive addon add hive-carto | grep -q 'already in' || { echo "second add not recognised"; fail=1; }
rm -f ~/hive-mcp/local.deps.edn

if [ -n "${HIVE_JOURNEY_TOKEN:-}" ]; then
  echo "--- a real token is accepted and written"
  printf '%s\n' "$HIVE_JOURNEY_TOKEN" | hive store login || fail=1
  hive store login --check | grep -q 'gateway accepts' || fail=1
fi
exit $fail
