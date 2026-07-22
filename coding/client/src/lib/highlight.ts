import hljs from 'highlight.js/lib/common'

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

export function languageForPath(path: string): string {
  const extension = path.split('.').pop()?.toLowerCase() ?? ''
  const language = extensionLanguages[extension] ?? 'plaintext'
  return hljs.getLanguage(language) ? language : 'plaintext'
}

export function highlightCode(code: string, language: string): string {
  return hljs.highlight(code, { language }).value
}
