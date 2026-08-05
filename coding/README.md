# Or

Or is the product built on the reusable `agent` and `llm` libraries. Its
packages are product implementation details and are not a public SDK.

Or and the Go packages use the same version.

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
    ├── prompt/             Or system prompt
    ├── prompttemplate/     Prompt template discovery and expansion
    ├── skills/             Skill discovery and loading
    ├── tools/              Or tools and local execution
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
import it. The `coding` product packages must not depend on `harness`.

## Prompt templates

Prompt templates are Markdown files that expand from slash commands. Or
loads user templates from `~/.or/prompts/*.md` and project templates from
`<workspace>/.or/prompts/*.md`. A project template replaces a user template
with the same filename-derived name.

For example, `.or/prompts/review.md` defines `/review`:

```markdown
---
description: Review working tree changes
argument-hint: "[focus]"
---
Review the current changes. Focus on ${ARGUMENTS:-bugs and regressions}.
```

Templates support `$1`, `$2`, `$@`, `$ARGUMENTS`, default values such as
`${1:-default}`, and slices such as `${@:2}` or `${@:2:3}`. The slash menu
shows the short command, argument hint, source, and description. Conversation
history keeps the short invocation while the expanded Markdown is sent to the
model as product-owned context.

Localized menu metadata is optional. Add `description-en`,
`description-zh-CN`, `argument-hint-en`, and `argument-hint-zh-CN` to make a
template follow Or's interface language. The original fields remain the
fallback for older and single-language templates.

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

Use the following command to create ad-hoc signed macOS distributables under
`coding/desktop/release`:

```sh
bun run package -- --mac --publish never \
  --config.mac.identity=- \
  --config.mac.hardenedRuntime=false
```

Repository `vX.Y.Z` tags build Apple Silicon and Intel Mac installers. See
[`RELEASING.md`](../RELEASING.md).

The desktop and standalone shells share provider settings, sessions and
transcripts under `~/.or/coding`. Set `OR_DATA_DIR` to use another location.
The desktop shell is single-instance: launching it again restores and focuses
the existing window.
