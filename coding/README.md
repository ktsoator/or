# Coding

Coding is the product built on the reusable `agent` and `llm` libraries. Its
packages are product implementation details and are not a public SDK.

Coding and the Go packages use the same version.

## Layout

```text
coding/
├── client/                 React client
├── desktop/                Electron desktop shell
├── cmd/coding/             Standalone API entry point
└── internal/
    ├── app/                Composition root and process startup
    ├── engine/             One stateful coding-agent session
    ├── conversation/       Product conversation lifecycle and queueing
    ├── httpapi/            HTTP and SSE delivery
    ├── transcript/         Transcript model and persistence
    ├── compaction/         Context compaction
    ├── permission/         Tool-call approval policy
    ├── prompt/             Coding system prompt
    ├── skills/             Skill discovery and loading
    ├── tools/              Coding tools and local execution
    ├── provider/           Provider settings
    ├── workspace/          Workspace registry and scratch directories
    ├── usage/              Usage ledger
    └── config/             Process startup configuration
```

## Dependency direction

```text
client -> HTTP/SSE -> httpapi -> conversation -> engine -> agent -> llm
                         ^             |
                         |             +-> coding product packages
                         +--- app creates and connects all services
```

Both `cmd/coding` and `desktop` host the reusable runtime assembled by
`internal/app`. Product policy stays inside `coding`; `agent` and `llm` must not
import it. Coding must not depend on `harness`.

## Desktop

Electron supervises a dedicated Go sidecar on a random loopback port. The
sidecar serves both the React build and `/api`, so browser and desktop clients
keep the same relative HTTP/SSE contract. Every request requires a per-launch
HttpOnly session cookie installed by Electron before the first navigation.

The right-side Browser is rendered with Electron `<webview>` elements. React
owns their layout and tab lifecycle, so menus and dialogs can compose above a
page without hiding a separate native child view. A renderer-side registry owns
navigation revisions, history state, failure reporting, and bounded read-only
inspection.

Public HTTP(S), localhost, and workspace preview pages currently share the
desktop session. Dedicated webview partitions, workspace request isolation,
permission policy, and attach-time validation remain required before treating
untrusted workspace content as isolated.

Run the desktop app in development:

```sh
cd coding/desktop
bun install
bun run dev
```

Build an unpacked application for the current platform:

```sh
cd coding/desktop
bun run package:dir
```

Use `bun run package -- --mac --publish never` to create macOS distributables
under `coding/desktop/release`.

Repository `vX.Y.Z` tags build Apple Silicon and Intel Mac installers. See
[`RELEASING.md`](../RELEASING.md).

The desktop and standalone shells share provider settings, sessions and
transcripts under `~/.or/coding`. Set `OR_DATA_DIR` to use another location.
The desktop shell is single-instance: launching it again restores and focuses
the existing window.
