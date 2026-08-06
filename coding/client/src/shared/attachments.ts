import type { PendingFile, PendingImage } from '@/types'

export const maxImages = 4
export const maxImageBytes = 10 * 1024 * 1024
export const maxImagesBytes = 20 * 1024 * 1024

const imageMIMETypes = new Set([
  'image/gif',
  'image/jpeg',
  'image/png',
  'image/webp',
])

export type ImageValidationError = 'count' | 'type' | 'file_size' | 'total_size'

export function validateImageFiles(
  current: Pick<PendingImage, 'size'>[],
  selected: Pick<File, 'size' | 'type'>[],
): ImageValidationError | undefined {
  if (current.length + selected.length > maxImages) return 'count'
  if (selected.some((file) => !imageMIMETypes.has(file.type))) return 'type'
  if (selected.some((file) => file.size > maxImageBytes)) return 'file_size'
  const total =
    current.reduce((sum, image) => sum + image.size, 0) +
    selected.reduce((sum, file) => sum + file.size, 0)
  return total > maxImagesBytes ? 'total_size' : undefined
}

export function readImage(file: File): Promise<PendingImage> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onerror = () => reject(reader.error)
    reader.onload = () => {
      const result = typeof reader.result === 'string' ? reader.result : ''
      const comma = result.indexOf(',')
      if (comma < 0) {
        reject(new Error('invalid image data'))
        return
      }
      resolve({
        id: `${file.name}-${file.lastModified}-${crypto.randomUUID()}`,
        name: file.name,
        size: file.size,
        mimeType: file.type,
        data: result.slice(comma + 1),
      })
    }
    reader.readAsDataURL(file)
  })
}

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
