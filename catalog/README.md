# Or Catalog

`catalog` means a curated directory or collection. This directory groups
resources recommended or distributed by the Or project; it is not a runtime
configuration directory and browsing it does not install or enable anything.

## Collections

| Collection | Contents | Runtime behavior |
|---|---|---|
| [Skills](skills/README.md) | Self-contained Agent Skill packages curated for redistribution | Install a selected package into a standard `.agents/skills` location before use |
| [MCP](mcp/README.md) | Recommended MCP servers with Or configuration examples | Review an entry and add its configuration to Or before connecting |
| [CLI](cli/README.md) | Lightweight recommendations for useful command-line tools | Follow the upstream installation documentation when a tool is wanted |

Each collection keeps its own format and contribution requirements. Sharing a
top-level directory makes these resources easier to discover without implying
that Skills, MCP servers, and CLI tools have the same installation or security
model.

## Principles

- Prefer authoritative upstream sources and actively maintained projects.
- Preserve attribution and licensing for redistributed content.
- Do not include credentials, private endpoints, local configuration, caches,
  or generated output.
- Document security-relevant behavior and require explicit user action before
  installing software, enabling a server, or executing bundled scripts.
