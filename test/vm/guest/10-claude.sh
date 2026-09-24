# Stage: the customer has Claude Code. Not ours to test, but everything after needs it.
curl -fsSL https://claude.ai/install.sh | bash >/dev/null 2>&1
export PATH="$HOME/.local/bin:$PATH"
claude --version
