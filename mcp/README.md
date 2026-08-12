# Or MCP Catalog

This directory contains MCP server configurations curated for Or. Each entry
describes one upstream server, the Or-compatible configuration used to connect
to it, its prerequisites, and the security decisions a user should review
before adding it.

Catalog entries are templates, not installed configuration. Or continues to
load enabled servers only from its private configuration file:

- default: `~/.or/coding/mcp.json`
- custom data directory: `$OR_DATA_DIR/mcp.json`

The catalog must never contain credentials. Templates reference existing
process environment variables with `${env:NAME}` placeholders. A future
installer should open the existing MCP server editor with the template filled
in and require explicit confirmation before saving or connecting.

## Included servers

| Server | Transport | Authentication | Upstream revision |
|---|---|---|---|
| [GitHub](servers/github/README.md) | Streamable HTTP | Personal access token | [`github/github-mcp-server@eff4c3c`](https://github.com/github/github-mcp-server/tree/eff4c3c041742426f417f7c2247b96bbf6d60b69) |

## Layout

```text
mcp/
|-- README.md
|-- README.zh.md
`-- servers/
    `-- <name>/
        |-- manifest.json
        `-- README.md
```

`manifest.json` is the machine-readable source for a future catalog UI. Its
`server.config` object uses the same fields accepted by Or's MCP editor and
`mcp.json` configuration. The adjacent README explains setup choices that do
not belong in executable configuration.

## Contribution requirements

- Use a stable lowercase catalog ID for the directory and `id` field.
- Link to an authoritative upstream repository and pin the revision reviewed.
- Record the upstream license, but do not copy upstream code unless Or has a
  reason to distribute it and the required license and notices are included.
- Include only stdio or Streamable HTTP configurations supported by Or.
- Use `${env:NAME}` for secrets. Never include tokens, passwords, cookies, or
  private endpoints, even as examples.
- Prefer least-privilege and read-only defaults where the server supports them.
- Describe network, process, filesystem, and write capabilities honestly.
- Review all commands, container images, package names, URLs, and arguments at
  the pinned upstream revision before accepting an entry.
- Catalog installation must remain a reviewed user action. A catalog entry must
  never connect, execute a command, or modify `mcp.json` merely by being viewed.

