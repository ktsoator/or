const providerNames: Record<string, string> = {
  anthropic: 'Anthropic',
  deepseek: 'DeepSeek',
  google: 'Google',
  minimax: 'MiniMax',
  'minimax-cn': 'MiniMax CN',
  'kimi-coding': 'Kimi Coding',
  moonshotai: 'Moonshot AI',
  'moonshotai-cn': 'Moonshot AI CN',
  openai: 'OpenAI',
  'opencode-go': 'OpenCode',
  openrouter: 'OpenRouter',
  xai: 'xAI',
  xiaomi: 'Xiaomi',
  'xiaomi-token-plan-ams': 'Xiaomi MiMo AMS',
  'xiaomi-token-plan-cn': 'Xiaomi MiMo CN',
  'xiaomi-token-plan-sgp': 'Xiaomi MiMo SGP',
  zai: 'Z.AI',
  'zai-coding-cn': 'Z.AI Coding CN',
}

export function providerName(provider: string): string {
  return (
    providerNames[provider] ??
    provider
      .split('-')
      .filter(Boolean)
      .map((part) => part[0]?.toUpperCase() + part.slice(1))
      .join(' ')
  )
}
