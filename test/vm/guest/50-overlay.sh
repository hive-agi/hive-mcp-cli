# Stage: a personal overlay in ~/hive-mcp/local.deps.edn reaches the host classpath.
# This is the seam a subscriber's licensed coordinates go through, so it is
# checked with a harmless public library that the starter pack does not carry,
# plus the hive-store repo entry exactly as the store's setup page renders it.
cd ~/hive-mcp || exit 1
cat > local.deps.edn <<'EDN'
;; journey probe: an overlay entry the starter pack does not carry
{:mvn/repos {"hive-store" {:url "https://store.hive-mcp.com/maven"}}
 :deps {org.clojure/data.csv {:mvn/version "1.1.0"}}}
EDN
( sleep 150 ) | timeout 170 bin/hive-mcp-foss >/dev/null 2>/tmp/overlay.err &
# The HOST's JVM, recognised by a starter-pack jar on its classpath (the project's
# own dirs appear relative, as "src"). The launcher starts a short-lived JVM first,
# to merge the overlay, which must not be mistaken for it.
for i in $(seq 1 150); do
  cp="$(ps -ww -eo args | grep -E 'java .*hive-datahike' | grep -v grep | head -1)"
  [ -n "$cp" ] && break
  sleep 1
done
wait
rm -f local.deps.edn
grep -E 'merging|could not merge' /tmp/overlay.err
echo "$cp" | grep -q 'data.csv-1.1.0.jar' && echo "overlay dep on classpath" || { echo "overlay dep NOT on classpath"; exit 1; }
echo "$cp" | grep -q 'hive-datahike' && echo "starter dep still on classpath" || { echo "starter dep LOST"; exit 1; }
