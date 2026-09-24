# Stage: a NEW login shell, in a project that is not $HOME, asks Claude Code whether
# the hive server answers. `claude mcp list` runs a real MCP initialize against it.
export PATH="$HOME/.local/bin:$PATH"
mkdir -p ~/myapp && cd ~/myapp
# An interactive shell, as a customer's terminal is: Ubuntu's .bashrc returns
# early for non-interactive ones, before the line setup appends.
[ -n "$(bash -ic 'echo $HIVE_MCP_DIR' 2>/dev/null)" ] || { echo "HIVE_MCP_DIR not exported to a new shell"; exit 1; }
id -nG | grep -qw docker || { echo "customer is not in the docker group"; exit 1; }
out="$(MCP_TIMEOUT=600000 claude mcp list 2>&1)"
echo "$out"
hive doctor 2>&1 | grep -E '✗|Summary'
for p in ~/.gitconfig ~/.clojure/deps.edn; do
  [ -d "$p" ] && { echo "$p is a directory (a docker bind mount created it)"; exit 1; }
done
echo "$out" | grep -q '^hive: .*Connected'
