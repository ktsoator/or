const extensionLanguages: Record<string, string> = {
  c: 'c',
  cc: 'cpp',
  cpp: 'cpp',
  css: 'css',
  go: 'go',
  h: 'c',
  hpp: 'cpp',
  html: 'xml',
  java: 'java',
  js: 'javascript',
  json: 'json',
  jsx: 'javascript',
  md: 'markdown',
  py: 'python',
  rs: 'rust',
  sh: 'bash',
  ts: 'typescript',
  tsx: 'typescript',
  xml: 'xml',
  yaml: 'yaml',
  yml: 'yaml',
}

export type SyntaxHighlighter = {
  getLanguage: (name: string) => unknown
  highlight: (code: string, options: { language: string }) => { value: string }
}

let loadedHighlighter: SyntaxHighlighter | undefined
let highlighterPromise: Promise<SyntaxHighlighter> | undefined

export function currentSyntaxHighlighter(): SyntaxHighlighter | undefined {
  return loadedHighlighter
}

export function loadSyntaxHighlighter(): Promise<SyntaxHighlighter> {
  if (loadedHighlighter) return Promise.resolve(loadedHighlighter)
  highlighterPromise ??= import('./highlightRuntime').then(({ syntaxHighlighter }) => {
    loadedHighlighter = syntaxHighlighter
    return syntaxHighlighter
  })
  return highlighterPromise
}

export function languageForPath(path: string): string {
  const extension = path.split('.').pop()?.toLowerCase() ?? ''
  return extensionLanguages[extension] ?? 'plaintext'
}

export function highlightLanguage(
  language: string | undefined,
  highlighter?: SyntaxHighlighter,
): string {
  const requested = language?.trim().split(/\s+/)[0]?.toLowerCase() || 'plaintext'
  if (!highlighter) return requested
  return highlighter.getLanguage(requested) ? requested : 'plaintext'
}

export function escapeHTML(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

export function highlightCode(
  code: string,
  language: string,
  highlighter?: SyntaxHighlighter,
): string {
  if (!highlighter) return escapeHTML(code)
  const resolvedLanguage = highlightLanguage(language, highlighter)
  return highlighter.highlight(code, { language: resolvedLanguage }).value
}

export function containsHighlightableCode(source: string): boolean {
  return /(?:^|\n)(?: {0,3}(?:`{3,}|~{3,})| {4}\S)/.test(source)
}
