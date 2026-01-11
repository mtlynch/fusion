## Development

- Dev scripts should set `GOCACHE` to a writable path (default `GOCACHE=/tmp/go-cache`) to avoid permission issues.

## Run the app

- From `/workspace/fusion`, start the dev server with `dev-scripts/serve`.
- The app listens on `http://localhost:8080` by default.

## Playwright (MCP)

- Never try to install browsers through the Playwright MCP
- Use the MCP Playwright server tools to drive the UI (for example,
  `mcp__playwright__browser_navigate` -> `mcp__playwright__browser_snapshot`).
