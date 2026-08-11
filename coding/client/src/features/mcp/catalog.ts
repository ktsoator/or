import { apiURL } from '@/api'

export type MCPServerConfig = {
  disabled?: boolean
  command?: string
  args?: string[]
  env?: Record<string, string>
  cwd?: string
  url?: string
  headers?: Record<string, string>
  workspaces?: string[]
  timeoutSeconds?: number
}

export type MCPServerInfo = {
  name: string
  config: MCPServerConfig
  inScope?: boolean
  diagnostic?: string
}

export type MCPListResponse = {
  path: string
  servers: MCPServerInfo[]
}

export type MCPProbeTool = {
  name: string
  original: string
  title?: string
  description?: string
}

export type MCPProbeResult = {
  transport: 'stdio' | 'streamable_http'
  tools: MCPProbeTool[]
  latencyMs: number
}

type SaveServerRequest = {
  name: string
  previousName?: string
  config: MCPServerConfig
}

function workspaceQuery(workspacePath?: string): string {
  return workspacePath ? `?workspace=${encodeURIComponent(workspacePath)}` : ''
}

async function responseError(response: Response, fallback: string): Promise<Error> {
  const body = (await response.json().catch(() => ({}))) as { error?: string }
  return new Error(body.error || fallback)
}

export async function fetchMCPServers(
  workspacePath?: string,
  signal?: AbortSignal,
): Promise<MCPListResponse> {
  const response = await fetch(apiURL(`/mcp${workspaceQuery(workspacePath)}`), {
    cache: 'no-store',
    signal,
  })
  if (!response.ok) throw await responseError(response, 'failed to load MCP servers')
  return response.json() as Promise<MCPListResponse>
}

export async function saveMCPServer(
  request: SaveServerRequest,
  workspacePath?: string,
): Promise<MCPServerInfo> {
  const response = await fetch(apiURL(`/mcp/servers${workspaceQuery(workspacePath)}`), {
    method: 'PUT',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(request),
  })
  if (!response.ok) throw await responseError(response, 'failed to save MCP server')
  return response.json() as Promise<MCPServerInfo>
}

export async function deleteMCPServer(name: string): Promise<void> {
  const response = await fetch(apiURL(`/mcp/servers/${encodeURIComponent(name)}`), {
    method: 'DELETE',
  })
  if (!response.ok) throw await responseError(response, 'failed to delete MCP server')
}

export async function testMCPServer(
  name: string,
  workspacePath?: string,
  signal?: AbortSignal,
): Promise<MCPProbeResult> {
  const response = await fetch(
    apiURL(`/mcp/servers/${encodeURIComponent(name)}/test${workspaceQuery(workspacePath)}`),
    { method: 'POST', signal },
  )
  if (!response.ok) throw await responseError(response, 'MCP connection test failed')
  return response.json() as Promise<MCPProbeResult>
}

export function mcpTransport(config: MCPServerConfig): 'stdio' | 'http' {
  return config.command ? 'stdio' : 'http'
}

export function mcpEndpoint(config: MCPServerConfig): string {
  if (config.command) return [config.command, ...(config.args ?? [])].join(' ')
  return config.url ?? ''
}
