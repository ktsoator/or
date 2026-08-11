# Or

Or is the product built on the reusable `agent` and `llm` libraries. Its
packages are product implementation details and are not a public SDK.

Or and the Go packages use the same version.

## Layout

```text
coding/
├── client/                 React client
├── desktop/                Electron desktop shell
├── cmd/coding-desktop/     Authenticated desktop sidecar
└── internal/
    ├── app/                Product runtime composition root
    ├── engine/             One stateful coding-agent session
    ├── conversation/       Product conversation lifecycle and queueing
    ├── httpapi/            HTTP and SSE delivery
    ├── transcript/         Transcript model and persistence
    ├── compaction/         Context compaction
    ├── permission/         Tool-call approval policy
    ├── prompt/             Or system prompt
    ├── skills/             Skill discovery and loading
    ├── tools/              Or tools and local execution
    ├── provider/           Provider settings
    ├── workspace/          Workspace registry and scratch directories
    └── usage/              Usage ledger
```

## Dependency direction

```text
client -> HTTP/SSE -> httpapi -> conversation -> engine -> agent -> llm
                         ^             |
                         |             +-> coding product packages
                         +--- app creates and connects all services
```

Electron supervises `cmd/coding-desktop`, which hosts the runtime assembled by
`internal/app`. Product policy stays inside `coding`; `agent` and `llm` must not
import it. The `coding` product packages must not depend on `harness`.

## Agent Skills

Or implements the open [Agent Skills specification](https://agentskills.io/specification).
It loads user skills from `~/.agents/skills/<name>/SKILL.md` and workspace skills
from `<workspace>/.agents/skills/<name>/SKILL.md`. A workspace skill replaces a
user skill with the same name.

`SKILL.md` must contain standard YAML frontmatter followed by Markdown
instructions:

```markdown
---
name: code-review
description: Review code for defects and regressions. Use when asked to review changes.
---

# Code review

Inspect the diff and report findings by severity.
```

Or validates the standard `name`, `description`, `license`, `compatibility`,
`metadata`, and `allowed-tools` fields. Unknown top-level fields are rejected.
The Markdown body is loaded unchanged; Skill files do not support argument
substitution. Relative file references resolve from the skill directory.

Type `/` in the composer to search built-in commands and Skills in one catalog.
Selecting a Skill creates a typed Skill reference. Or can also activate a Skill
automatically when its description matches the task. Activated instructions are
kept as protected session context across compaction. The
experimental `allowed-tools` field is preserved but never bypasses Or's normal
permission policy.

## MCP servers

Or can discover and call tools from configured Model Context Protocol servers.
See [MCP configuration](MCP.md) for the supported stdio and Streamable HTTP
transports, workspace scoping, environment references, and security model.

## Desktop

Electron supervises a dedicated Go sidecar on a random loopback port. The
sidecar serves both the React build and `/api`, so the renderer uses one relative
HTTP/SSE contract. Every request requires a per-launch HttpOnly session cookie
installed by Electron before the first navigation.

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

Provider settings, sessions and transcripts live under `~/.or/coding`. Set
`OR_DATA_DIR` to use another location. The desktop shell is single-instance:
launching it again restores and focuses the existing window.
