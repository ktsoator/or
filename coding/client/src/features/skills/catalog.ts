import { apiURL } from '@/api'

export type SkillEntry = {
  name: string
  description: string
  source: 'user' | 'project'
  dir: string
  path?: string
  disableModelInvocation: boolean
}

export type SkillDiagnostic = {
  path: string
  message: string
}

export type SkillsResponse = {
  user: SkillEntry[]
  project: SkillEntry[]
  diagnostics: SkillDiagnostic[]
}

export async function fetchSkills(
  workspacePath?: string,
  signal?: AbortSignal,
): Promise<SkillsResponse> {
  const query = workspacePath
    ? `?workspace=${encodeURIComponent(workspacePath)}`
    : ''
  const response = await fetch(apiURL(`/skills${query}`), {
    cache: 'no-store',
    signal,
  })
  if (!response.ok) throw new Error('failed to load skills')
  return response.json() as Promise<SkillsResponse>
}

export type SkillSlashQuery = {
  query: string
  argumentsText: string
}

// parseSkillSlashQuery recognizes the composer search shorthand `/name arguments`.
// Selection serializes the skill as a durable Markdown reference to SKILL.md.
export function parseSkillSlashQuery(draft: string): SkillSlashQuery | undefined {
  if (!draft.startsWith('/')) return undefined
  const token = draft.slice(1).match(/^[^\s]*/)?.[0] ?? ''
  if (token.includes(':')) return undefined
  return {
    query: token,
    argumentsText: draft.slice(token.length + 1).trimStart(),
  }
}

export function skillArgumentsFromDraft(draft: string): string {
  const slash = parseSkillSlashQuery(draft)
  if (slash) return slash.argumentsText
  const explicit = draft.trimStart().match(/^\/skill:\S+/)?.[0]
  return explicit ? draft.trimStart().slice(explicit.length).trimStart() : draft
}

export function buildSkillInvocation(
  skill: Pick<SkillEntry, 'name' | 'dir' | 'path'>,
  argumentsText: string,
): string {
  const path = skill.path || `${skill.dir.replace(/[\\/]$/, '')}/SKILL.md`
  const destination = /[\s()]/.test(path)
    ? `<${path.replaceAll('>', '%3E')}>`
    : path
  const command = `[$${skill.name}](${destination})`
  const trimmed = argumentsText.trim()
  return trimmed ? `${command} ${trimmed}` : command
}

export function displaySkillInvocation(text: string): string {
  const trimmed = text.trim()
  const match = trimmed.match(/^\/skill:([^\s]+)(?:\s+([\s\S]*))?$/)
  if (!match) return text
  const argumentsText = match[2]?.trim()
  const reference = `[$${match[1]}]()`
  return argumentsText ? `${reference} ${argumentsText}` : reference
}

export type SkillReference = {
  name: string
  path: string
  argumentsText: string
  markdown: string
}

export function parseSkillReference(text: string): SkillReference | undefined {
  const trimmed = text.trim()
  const match = trimmed.match(/^\[\$([^\]]+)\]\((?:<([^>]+)>|([^)]*))\)/)
  if (!match) return undefined
  return {
    name: match[1],
    path: match[2] ?? match[3] ?? '',
    argumentsText: trimmed.slice(match[0].length).trimStart(),
    markdown: match[0],
  }
}

export function serializeSkillReferenceCopy(
  reference: SkillReference,
  selectedText: string,
): string {
  const visibleLabel = reference.name
  if (selectedText.startsWith(visibleLabel)) {
    return reference.markdown + selectedText.slice(visibleLabel.length)
  }
  const storedLabel = `$${reference.name}`
  if (!selectedText.startsWith(storedLabel)) return selectedText
  return reference.markdown + selectedText.slice(storedLabel.length)
}

export function filterSkills(skills: SkillEntry[], query: string): SkillEntry[] {
  const normalized = query.trim().toLocaleLowerCase()
  if (!normalized) return skills
  return skills.filter(
    (skill) =>
      skill.name.toLocaleLowerCase().includes(normalized) ||
      skill.description.toLocaleLowerCase().includes(normalized),
  )
}
