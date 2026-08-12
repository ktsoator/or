# Or MCP Collection

This directory is a lightweight collection of MCP servers that are useful with
Or. It helps people discover trustworthy servers, understand what they provide,
and review what is required before connecting one.

This is not runtime configuration. Each entry is a short guide with an official
source, common use cases, an Or configuration example, authentication
requirements, and relevant security notes. Or loads enabled servers only from
its private `mcp.json` configuration.

## Included servers

| Server | Transport | Recommended for |
|---|---|---|
| [GitHub](github/README.md) | Streamable HTTP | Repositories, code search, issues, pull requests, and workflows |

## Layout

```text
catalog/mcp/
|-- README.md
|-- README.zh.md
`-- <name>/
    `-- README.md
```

## Contribution requirements

- Keep one server in each immediate child directory.
- Explain what the server provides, why it is recommended, and when it is
  useful.
- Link to an authoritative upstream source and record the documentation,
  license, and revision reviewed.
- Include only stdio or Streamable HTTP configurations supported by Or.
- Keep configuration examples compatible with Or and reference secrets through
  `${env:NAME}` placeholders.
- Never include tokens, passwords, cookies, private endpoints, or
  machine-specific setup.
- Describe authentication, network access, local execution, and write
  capabilities honestly; prefer least-privilege defaults where possible.
- Adding or connecting a server must remain an explicit user action.
