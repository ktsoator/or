export const composerFloatingPanelClass =
  'absolute right-0 bottom-[calc(100%+0.5rem)] left-0 z-[115] min-w-0 max-w-full max-h-[min(23rem,calc(100vh-2rem))] overflow-hidden rounded-[16px] border border-edge bg-canvas p-1.5 text-ink outline-none animate-[fade-in_100ms_ease-out]'

export const composerPreviewCommands = ['review', 'compact', 'continue', 'plan'] as const
export type ComposerPreviewCommand = (typeof composerPreviewCommands)[number]

export function previewSkillCommandCount(query: string): number {
  return query.trim() ? 0 : composerPreviewCommands.length
}

export function moveSuggestionIndex(
  current: number,
  count: number,
  direction: 'next' | 'previous',
): number {
  if (count <= 0) return 0
  const index = current >= 0 && current < count ? current : 0
  if (direction === 'previous') return index === 0 ? count - 1 : index - 1
  return index === count - 1 ? 0 : index + 1
}

export function parseExecutableComposerCommand(
  input: string,
): Extract<ComposerPreviewCommand, 'compact'> | undefined {
  return input.trim() === '/compact' ? 'compact' : undefined
}

export type PlanComposerCommand = {
  active: boolean
  message: string
}

export function parsePlanComposerCommand(input: string): PlanComposerCommand | undefined {
  const match = /^\s*\/plan(?:\s+([\s\S]*))?\s*$/.exec(input)
  if (!match) return undefined
  const argument = (match[1] ?? '').trim()
  if (argument.toLocaleLowerCase() === 'off') return { active: false, message: '' }
  return { active: true, message: argument }
}

export function parseComposerCatalogQuery(
  draft: string,
): { query: string; argumentsText: string } | undefined {
  if (!draft.startsWith('/')) return undefined
  const token = draft.slice(1).match(/^[^\s]*/)?.[0] ?? ''
  return {
    query: token,
    argumentsText: draft.slice(token.length + 1).trimStart(),
  }
}

export function skillSuggestionOptionID(suggestionsID: string, index: number): string {
  return `${suggestionsID}-${index}`
}
