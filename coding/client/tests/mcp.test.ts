import { describe, expect, test } from 'bun:test'
import {
  mcpEndpoint,
  mcpTransport,
  type MCPServerConfig,
} from '../src/features/mcp'

describe('MCP server presentation', () => {
  test('identifies stdio and renders its command line', () => {
    const config: MCPServerConfig = {
      command: 'npx',
      args: ['-y', '@modelcontextprotocol/server-everything'],
    }
    expect(mcpTransport(config)).toBe('stdio')
    expect(mcpEndpoint(config)).toBe(
      'npx -y @modelcontextprotocol/server-everything',
    )
  })

  test('identifies Streamable HTTP and renders its endpoint', () => {
    const config: MCPServerConfig = { url: 'https://mcp.example.com/mcp' }
    expect(mcpTransport(config)).toBe('http')
    expect(mcpEndpoint(config)).toBe('https://mcp.example.com/mcp')
  })
})
