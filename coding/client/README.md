# Or client

The React application consumes the Or product through relative `/api`
HTTP and SSE routes. The Electron sidecar serves the production build and API
from one authenticated loopback origin. During desktop development, Electron
starts Vite and injects the sidecar URL through `CODING_API_PROXY`.

## Development

Run the complete desktop application:

```sh
cd coding/desktop
bun install
bun run dev
```

Session metadata and transcripts are stored under `~/.or/coding`. Set
`OR_DATA_DIR` to use another location.

Client-only checks run from this directory:

```sh
cd coding/client
bun install
bun run test
bun run lint
bun run build
```

The right-side Browser has no web iframe fallback. In Electron it renders public
sites, localhost apps, and workspace HTML with `<webview>` elements managed by a
renderer-side registry. Public pages share a persistent browser partition;
workspace HTML uses an in-memory partition and a preview-only sidecar origin.
Browser-only Playwright tests use a test bridge; they do not provide a separately
deployable product mode.

Run the desktop-shell UI regression tests with a locally installed Chrome:

```sh
bun run test:ui
```

## Production build

```sh
cd coding/desktop
bun run build
```

The desktop build compiles the React renderer, Electron main process, and Go
sidecar together.
