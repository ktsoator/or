# Coding client

The React application consumes the Coding product through relative `/api`
HTTP and SSE routes. Vite proxies them to `http://localhost:8787` for standalone
browser development; the Wails shell mounts the same routes in-process.

## Development

Start the API from the repository root:

```sh
go run ./coding/cmd/coding
```

Sessions select their project directory in the browser. The directory
picker and ordinary Chats start from the current user's home directory, while
session metadata and transcripts are stored under `~/.or/coding`. Override that
storage location with `OR_DATA_DIR` or `-data-dir` when needed.

Start the client in another terminal:

```sh
cd coding/client
bun install
bun run dev
```

Open `http://localhost:5173`.

Set `CODING_API_PROXY` when the local API uses a different address.

Run the desktop-shell UI regression tests with a locally installed Chrome:

```sh
bun run test:ui
```

## Production build

```sh
cd coding/client
bun run build
```

The static application is written to `coding/client/dist`. Deploy that directory with a
static host and route `/api/*` to the Go service.

For a fully cross-origin deployment, build with the API origin and allow the
client origin in the Go process:

```sh
VITE_API_ORIGIN=https://api.example.com bun run build
go run ./coding/cmd/coding -client-origin https://app.example.com
```

`OR_CLIENT_ORIGIN` is the environment-variable equivalent of `-client-origin`.
