import { apiURL } from '@/api'

export type SkillEntry = {
  name: string
  description: string
  source: 'user' | 'project'
  dir: string
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
  const query = workspacePath ? `?workspace=${encodeURIComponent(workspacePath)}` : ''
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

// parseSkillSlashQuery recognizes the composer shorthand `/name arguments`.
// The durable/backend command remains `/skill:name arguments`.
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

export function buildSkillInvocation(name: string, argumentsText: string): string {
  const command = `/skill:${name}`
  const trimmed = argumentsText.trim()
  return trimmed ? `${command} ${trimmed}` : command
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
