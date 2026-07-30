import type { PendingFile } from './types'

export const maxTextFiles = 10
export const maxTextFileBytes = 256 << 10
export const maxTextFilesBytes = 512 << 10

const textExtensions = new Set([
  '.adoc',
  '.bash',
  '.c',
  '.cc',
  '.cfg',
  '.cjs',
  '.conf',
  '.cpp',
  '.cs',
  '.css',
  '.dart',
  '.env',
  '.erl',
  '.ex',
  '.exs',
  '.fish',
  '.go',
  '.gql',
  '.graphql',
  '.h',
  '.hcl',
  '.hpp',
  '.hrl',
  '.htm',
  '.html',
  '.ini',
  '.java',
  '.js',
  '.json',
  '.jsonc',
  '.jsx',
  '.kt',
  '.kts',
  '.less',
  '.lock',
  '.lua',
  '.md',
  '.mdx',
  '.mjs',
  '.php',
  '.proto',
  '.py',
  '.r',
  '.rb',
  '.rs',
  '.rst',
  '.scss',
  '.sh',
  '.sql',
  '.svelte',
  '.swift',
  '.tf',
  '.toml',
  '.ts',
  '.tsx',
  '.txt',
  '.vue',
  '.xml',
  '.yaml',
  '.yml',
  '.zsh',
])

const textMIMETypes = new Set([
  'application/json',
  'application/ld+json',
  'application/toml',
  'application/x-httpd-php',
  'application/x-javascript',
  'application/x-sh',
  'application/xhtml+xml',
  'application/xml',
])

const textFileNames = new Set([
  'dockerfile',
  'gemfile',
  'license',
  'makefile',
  'procfile',
  'readme',
])

export type TextFileValidationError =
  | 'count'
  | 'type'
  | 'file_size'
  | 'total_size'

export function validateTextFiles(
  current: Pick<PendingFile, 'size'>[],
  selected: Pick<File, 'name' | 'size' | 'type'>[],
): TextFileValidationError | undefined {
  if (current.length + selected.length > maxTextFiles) return 'count'
  if (selected.some((file) => !isSupportedTextFile(file.name, file.type))) return 'type'
  if (selected.some((file) => file.size > maxTextFileBytes)) return 'file_size'
  const total =
    current.reduce((sum, file) => sum + file.size, 0) +
    selected.reduce((sum, file) => sum + file.size, 0)
  return total > maxTextFilesBytes ? 'total_size' : undefined
}

export function isSupportedTextFile(name: string, mimeType: string): boolean {
  const normalizedMIME = mimeType.trim().toLocaleLowerCase()
  if (normalizedMIME.startsWith('text/') || textMIMETypes.has(normalizedMIME)) return true
  const normalizedName = name.toLocaleLowerCase()
  const dot = normalizedName.lastIndexOf('.')
  return (
    textFileNames.has(normalizedName) ||
    (dot >= 0 && textExtensions.has(normalizedName.slice(dot)))
  )
}

export async function readTextFile(file: File): Promise<PendingFile> {
  const content = new TextDecoder('utf-8', { fatal: true }).decode(await file.arrayBuffer())
  if (content.includes('\0')) throw new Error('binary file')
  return {
    id: crypto.randomUUID(),
    name: file.name,
    mimeType: file.type.trim().toLocaleLowerCase() || 'text/plain',
    size: file.size,
    file,
  }
}

export function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1 << 20) return `${Math.max(1, Math.round(bytes / 1024))} KB`
  return `${(bytes / (1 << 20)).toFixed(1)} MB`
}
