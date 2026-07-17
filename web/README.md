# Coding web

The React application is independent from the Go coding API. Vite proxies
`/api/*` to `http://localhost:8787` during development.

## Development

Start the API from the repository root:

```sh
go run ./coding/cmd/coding -web -cwd .
```

Start the front-end in another terminal:

```sh
cd web
bun install
bun run dev
```

Open `http://localhost:5173`.

Set `CODING_API_PROXY` when the local API uses a different address.

## Production build

```sh
cd web
bun run build
```

The static application is written to `web/dist`. Deploy that directory with a
static host and route `/api/*` to the Go service.

For a fully cross-origin deployment, build with the API origin and allow the
front-end origin in the Go process:

```sh
VITE_API_ORIGIN=https://api.example.com bun run build
go run ./coding/cmd/coding -web -web-origin https://app.example.com
```

`OR_WEB_ORIGIN` is the environment-variable equivalent of `-web-origin`.
