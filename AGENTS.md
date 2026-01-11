## Development

- Dev scripts should set `GOCACHE` to a writable path (default `GOCACHE=/tmp/go-cache`) to avoid permission issues.

## Run the app

- From `/workspace/fusion`, start the dev server with `dev-scripts/serve`.
- The app listens on `http://localhost:8080` by default.

## Playwright (MCP)

- Never try to install browsers through the Playwright MCP
- Replace `/opt/google/chrome/chrome` with a symlink to the correct `chrome` binary under `PLAYWRIGHT_BROWSERS_PATH` before running Playwright MCP:
  `ln -sf "$(find "$PLAYWRIGHT_BROWSERS_PATH" -type f -name chrome -path '*/chrome-linux/*' -print -quit)" /opt/google/chrome/chrome`
- Use the MCP Playwright server tools to drive the UI (for example,
  `mcp__playwright__browser_navigate` -> `mcp__playwright__browser_snapshot`).
