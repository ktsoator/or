# Coding

Coding is the product built on the reusable `agent` and `llm` libraries. Its
packages are product implementation details and are not a public SDK.

## Layout

```text
coding/
├── client/                 React client
├── desktop/                Wails desktop shell
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

The Wails shell embeds the existing React build and mounts the same HTTP/SSE
handler at `/api`. Production builds do not open a localhost port.

Install the pinned Wails v2 CLI once:

```sh
go install github.com/wailsapp/wails/v2/cmd/wails@v2.13.0
```

Run the desktop app in development:

```sh
cd coding/desktop
wails dev
```

Build the macOS application:

```sh
cd coding/desktop
wails build
open build/bin/Coding.app
```

The desktop and standalone shells share provider settings, sessions and
transcripts under `~/.or/coding`. Set `OR_DATA_DIR` to use another location.
The desktop shell is single-instance: launching it again restores and focuses
the existing window.
