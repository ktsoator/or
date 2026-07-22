# Coding

Coding is the product built on the reusable `agent` and `llm` libraries. Its
packages are product implementation details and are not a public SDK.

## Layout

```text
coding/
├── client/                 React client
├── cmd/coding/             Process entry point
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

`cmd/coding` only parses configuration and starts `internal/app`. Product policy
stays inside `coding`; `agent` and `llm` must not import it. Coding must not
depend on `harness`.
