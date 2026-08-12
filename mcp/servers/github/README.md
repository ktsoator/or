# GitHub MCP Server

GitHub's official MCP server exposes repository, code search, issue, pull
request, user, and workflow operations. This catalog entry uses GitHub's hosted
Streamable HTTP endpoint, so it does not start a local process.

- Upstream: <https://github.com/github/github-mcp-server>
- Reviewed revision: [`eff4c3c041742426f417f7c2247b96bbf6d60b69`](https://github.com/github/github-mcp-server/tree/eff4c3c041742426f417f7c2247b96bbf6d60b69)
- License: MIT
- Endpoint: `https://api.githubcopilot.com/mcp/`

## Before adding

Or does not currently implement MCP OAuth. This template therefore expects a
GitHub personal access token in the environment variable
`GITHUB_PERSONAL_ACCESS_TOKEN`.

Create a fine-grained token that can access only the repositories required for
your work. Grant only the read or write permissions needed for the operations
you intend the agent to perform. Do not put the token itself in `manifest.json`
or `mcp.json`.

The desktop application must inherit the variable when it starts. Set
`GITHUB_PERSONAL_ACCESS_TOKEN` in the shell or secret manager used to launch Or;
verify that the variable exists without printing its value:

```sh
test -n "$GITHUB_PERSONAL_ACCESS_TOKEN"
```

## Or configuration

The catalog template corresponds to this entry in Or's private `mcp.json`:

```json
{
  "version": 1,
  "mcpServers": {
    "github": {
      "url": "https://api.githubcopilot.com/mcp/",
      "headers": {
        "Authorization": "Bearer ${env:GITHUB_PERSONAL_ACCESS_TOKEN}"
      },
      "timeoutSeconds": 20
    }
  }
}
```

After saving the server in Or, use **Test connection** and inspect the tools it
discovers before making it available to conversations.

## Security review

- The server receives the configured bearer token over HTTPS.
- Available tools and their effects are controlled by both the server and the
  token's repository permissions.
- With write permissions, tools may change repositories, issues, pull requests,
  releases, workflows, and other GitHub data.
- Or's Ask and Auto edit permission modes still request approval for remote MCP
  tool calls; Full access permits them without an additional prompt.
- Rotate or revoke the token from GitHub if it is exposed or no longer needed.
