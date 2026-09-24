# Contributing

The relay and web app are TypeScript on Cloudflare Workers, the CLI is Go, and the native apps live under `ios/` and `macos/`. `scripts/test-all.sh` runs every suite; the sections below describe the browser suite and the release procedure. TestFlight distribution is described in [AGENTS.md](AGENTS.md).

## Web tests

```sh
npm ci
npx playwright install --with-deps chromium
npm run check
npm run test:e2e
```

The browser suite requires Go and runs Chromium against a local Wrangler Worker on
`127.0.0.1:8790`. It builds the real CLI and uses its MCP interface for session
creation, events, and responses; it does not mock the relay or seed browser storage.
Each test starts with isolated browser storage. The phone, portrait-tablet, and
landscape-tablet projects check empty state, device management, joining sessions,
notification dismissal, answers, feedback, device addition/removal, and card layout.
These are Chromium viewport tests, not physical iPhone or Safari tests.

The `web` job runs both suites on pull requests. Stage screenshots and the HTML
report are retained as the `web-e2e-results` artifact; failures also retain browser
traces. Locally, use `npx playwright show-report` to inspect the same report.
`scripts/test-all.sh` includes the browser suite as well.

## Releases

GoReleaser builds archives for Linux, macOS, and Windows on amd64 and arm64. Pushing a semantic version tag creates a GitHub Release with those archives and `checksums.txt`:

```sh
git tag v2026.8.0
git push origin v2026.8.0
```
