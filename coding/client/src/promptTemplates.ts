import { apiURL } from '@/api'
import type { Locale } from '@/i18n'

export type PromptTemplateEntry = {
  name: string
  description: string
  descriptions?: Partial<Record<Locale, string>>
  argumentHint: string
  argumentHints?: Partial<Record<Locale, string>>
  source: 'user' | 'project'
  path: string
}

export function localizePromptTemplate(
  template: PromptTemplateEntry,
  locale: Locale,
): PromptTemplateEntry {
  const description = template.descriptions?.[locale] || template.description
  const argumentHint = template.argumentHints?.[locale] || template.argumentHint
  if (
    description === template.description &&
    argumentHint === template.argumentHint
  ) {
    return template
  }
  return {
    ...template,
    description,
    argumentHint,
  }
}

export type PromptTemplateDiagnostic = {
  path: string
  message: string
}

export type PromptTemplatesResponse = {
  user: PromptTemplateEntry[]
  project: PromptTemplateEntry[]
  diagnostics: PromptTemplateDiagnostic[]
}

export type PromptTemplateDetail = PromptTemplateEntry & {
  content: string
}

export async function fetchPromptTemplates(
  workspacePath?: string,
  signal?: AbortSignal,
): Promise<PromptTemplatesResponse> {
  const query = workspacePath ? `?workspace=${encodeURIComponent(workspacePath)}` : ''
  const response = await fetch(apiURL(`/prompt-templates${query}`), {
    cache: 'no-store',
    signal,
  })
  if (!response.ok) throw new Error('failed to load prompt templates')
  return response.json() as Promise<PromptTemplatesResponse>
}

export async function fetchPromptTemplateDetail(
  name: string,
  workspacePath?: string,
  signal?: AbortSignal,
): Promise<PromptTemplateDetail> {
  const query = workspacePath ? `?workspace=${encodeURIComponent(workspacePath)}` : ''
  const response = await fetch(
    apiURL(`/prompt-templates/${encodeURIComponent(name)}${query}`),
    { cache: 'no-store', signal },
  )
  if (!response.ok) throw new Error('failed to load prompt template')
  return response.json() as Promise<PromptTemplateDetail>
}

export function filterPromptTemplates(
  templates: PromptTemplateEntry[],
  query: string,
): PromptTemplateEntry[] {
  const normalized = query.trim().toLocaleLowerCase()
  if (!normalized) return templates
  return templates.filter(
    (template) =>
      template.name.toLocaleLowerCase().includes(normalized) ||
      template.description.toLocaleLowerCase().includes(normalized),
  )
}

export function buildPromptTemplateInvocation(
  name: string,
  argumentsText: string,
): string {
  const command = `/${name}`
  const trimmed = argumentsText.trim()
  return trimmed ? `${command} ${trimmed}` : command
}

export type PromptTemplateInvocation = {
  name: string
  argumentsText: string
}

export function promptTemplateArgumentsText(text: string, name: string): string {
  const trimmed = text.trim()
  const command = `/${name}`
  if (!trimmed.startsWith(command)) return ''
  const rest = trimmed.slice(command.length)
  if (rest && !/^\s/.test(rest)) return ''
  return rest.trim()
}

export function parsePromptTemplateInvocation(
  text: string,
): PromptTemplateInvocation | undefined {
  const match = text.trim().match(/^\/([^\s/]+)(?:\s+([\s\S]*))?$/)
  if (!match) return undefined
  return {
    name: match[1],
    argumentsText: match[2]?.trim() ?? '',
  }
}

export function serializePromptTemplateInvocationCopy(
  invocation: PromptTemplateInvocation,
  selectedText: string,
): string {
  if (!selectedText.startsWith(invocation.name)) return selectedText
  return `/${invocation.name}${selectedText.slice(invocation.name.length)}`
}
