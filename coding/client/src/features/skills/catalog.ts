import { apiURL } from '@/api'

export type SkillEntry = {
  name: string
  description: string
  license?: string
  compatibility?: string
  metadata?: Record<string, string>
  allowedTools?: string
  source: 'user' | 'project'
  dir: string
  path?: string
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

export type SkillMentionQuery = {
  query: string
  promptText: string
}

// parseSkillMentionQuery recognizes the composer search shorthand `$name prompt`.
// Selection serializes the skill as a durable Markdown reference to SKILL.md.
export function parseSkillMentionQuery(draft: string): SkillMentionQuery | undefined {
  if (!draft.startsWith('$')) return undefined
  const token = draft.slice(1).match(/^[^\s]*/)?.[0] ?? ''
  return {
    query: token,
    promptText: draft.slice(token.length + 1).trimStart(),
  }
}

export function skillPromptFromDraft(draft: string): string {
  return parseSkillMentionQuery(draft)?.promptText ?? draft
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
