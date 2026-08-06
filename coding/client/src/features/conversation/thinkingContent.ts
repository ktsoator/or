export type ThinkingContent = {
  title?: string
  body: string
}

const leadingTitle = /^[\t ]*\*\*(.+?)\*\*[\t ]*(?:\r?\n|$)/

// Reasoning summaries commonly start with a bold-only Markdown line. Promote
// it only after the closing marker arrives, so partial streams remain stable.
export function parseThinkingContent(text: string): ThinkingContent {
  const match = leadingTitle.exec(text)
  if (!match) return { body: text }

  const title = match[1]?.trim()
  if (!title) return { body: text }

  return {
    title,
    body: text.slice(match[0].length).replace(/^(?:[\t ]*\r?\n)+/, ''),
  }
}
