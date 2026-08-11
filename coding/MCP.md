# MCP configuration

Or can use tools exposed by Model Context Protocol servers over stdio or
Streamable HTTP. MCP connections belong to one loaded coding session and close
when that session is unloaded or the application exits.

## Internal boundaries

The integration keeps MCP protocol concerns separate from Or product concerns:

- `internal/mcpclient` owns configuration, transports, protocol sessions, tool
  discovery, and protocol-native tool calls and results. It does not depend on
  Or's agent, LLM, permission, or tool packages.
- `internal/mcpbridge` adapts discovered MCP tools to Or tool definitions,
  provider-safe names, permission accesses, and model-facing results.
- `internal/httpapi` and the client MCP page own configuration delivery and
  presentation. The engine only assembles the client session and bridge output.

## Configuration file

Use **MCP servers** in the application sidebar to add, edit, enable, test, or
remove servers. The page writes `mcp.json` in Or's data directory:

- default: `~/.or/coding/mcp.json`
- custom data directory: `$OR_DATA_DIR/mcp.json`

The configuration is deliberately product-owned rather than read from a
workspace. Opening an untrusted repository must not automatically start a
command supplied by that repository.

```json
{
  "version": 1,
  "mcpServers": {
    "project-files": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "${workspace}"
      ],
      "workspaces": [
        "/Users/example/src/project"
      ]
    },
    "internal-tools": {
      "url": "https://mcp.example.com/mcp",
      "headers": {
        "Authorization": "Bearer ${env:INTERNAL_MCP_TOKEN}"
      },
      "timeoutSeconds": 20
    }
  }
}
```

Each server must configure exactly one transport:

- `command`, plus optional `args`, `env`, and `cwd`, starts a stdio server.
- `url`, plus optional `headers`, connects with Streamable HTTP.

Other fields:

- `disabled`: keeps the entry in the status output without connecting it.
- `workspaces`: exact absolute workspace roots where the server is available.
  Omitting it makes the server global.
- `timeoutSeconds`: connection and initial tool-discovery timeout. The default
  is 15 seconds.

String values in commands, arguments, environment values, URLs, headers,
working directories, and workspace scopes support:

- `${workspace}`: the active session workspace.
- `${env:NAME}`: an existing process environment variable. A missing variable
  disables that server and appears in its diagnostic.
- `~` at the start of `cwd` and workspace scope paths.

Changes take effect the next time a conversation runtime loads. Closing and
reopening an idle conversation is enough; the application does not need to be
restarted. The page's connection test opens a temporary connection immediately,
discovers its tools, and closes it without changing a loaded conversation.

## Tool names and permissions

MCP tools are advertised as `mcp__<server>__<tool>`, with unsupported name
characters normalized and long names shortened deterministically. This avoids
collisions between servers and stays within provider tool-name limits.

MCP calls use Or's existing permission modes:

- stdio tools declare process execution access;
- Streamable HTTP tools declare network access;
- Full access allows them without prompting; Ask and Auto edit require
  approval for each call.

When a configuration exists, Or also provides the internal `mcp_status` tool.
It reports connected, disabled, out-of-scope, and failed servers without
including configured environment values or HTTP headers.

## Security

Treat `mcp.json` as executable configuration. A stdio entry starts the command
under the current user account, and MCP tools may have effects that their JSON
schemas do not describe. Only configure servers you trust.

Prefer `${env:NAME}` references over literal credentials. Or expands these
values only while building a connection and does not include headers or
environment values in status diagnostics. A stdio server inherits only a small
baseline environment needed to run commands; additional values must be declared
in its `env` map. The desktop authentication token is never inherited. HTTP
headers are attached only to requests for the configured endpoint origin,
including after redirects.

Tool text output is capped at 30,000 bytes and image output at 20 MiB per call
to prevent one server response from filling the model context or application
memory.

## Current scope

This first integration supports MCP tools. Resources, prompts, elicitation,
legacy HTTP+SSE servers, OAuth discovery, and live tool-list changes are not yet
projected into Or. Reopen a conversation after a server changes its tool list.
