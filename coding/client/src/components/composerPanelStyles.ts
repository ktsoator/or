export const composerFloatingPanelClass =
  'absolute right-0 bottom-[calc(100%+0.5rem)] left-0 z-[115] max-h-[min(23rem,calc(100vh-2rem))] overflow-hidden rounded-[16px] border border-edge bg-canvas p-1.5 text-ink shadow-[0_18px_50px_-24px_rgba(28,25,23,0.5)] outline-none animate-[fade-in_100ms_ease-out]'

export const skillSuggestionsID = 'composer-skill-suggestions'
export const composerPreviewCommands = [
  'review',
  'compact',
  'continue',
  'plan',
] as const
export type ComposerPreviewCommand = (typeof composerPreviewCommands)[number]

export function previewSkillCommandCount(query: string): number {
  return query.trim() ? 0 : composerPreviewCommands.length
}

export function parseExecutableComposerCommand(
  input: string,
): Extract<ComposerPreviewCommand, 'compact'> | undefined {
  return input.trim() === '/compact' ? 'compact' : undefined
}

export function skillSuggestionOptionID(index: number): string {
  return `${skillSuggestionsID}-${index}`
}
