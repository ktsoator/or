import type { SessionSummary, WorkspaceSummary } from './types'

export const DEFAULT_SIDEBAR_WIDTH = 240
export const MIN_SIDEBAR_WIDTH = 206
export const MAX_SIDEBAR_WIDTH = 338

export type WorkspaceSessionGroup = {
  path: string
  name: string
  sessions: SessionSummary[]
}

export function clampSidebarWidth(width: number) {
  return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, width))
}

export function parsePinnedSessionIDs(value: string | null): string[] {
  try {
    const parsed: unknown = JSON.parse(value ?? '[]')
    return Array.isArray(parsed)
      ? parsed.filter((id): id is string => typeof id === 'string')
      : []
  } catch {
    return []
  }
}

export function pinnedFirst(items: SessionSummary[], pinned: Set<string>): SessionSummary[] {
  return [...items].sort(
    (left, right) => Number(pinned.has(right.id)) - Number(pinned.has(left.id)),
  )
}

export function groupSidebarSessions(
  sessions: SessionSummary[],
  workspaces: WorkspaceSummary[],
  pinned: Set<string>,
) {
  const groups = new Map<string, WorkspaceSessionGroup>()
  for (const workspace of workspaces) {
    groups.set(workspace.path, {
      path: workspace.path,
      name: workspace.name,
      sessions: [],
    })
  }
  for (const session of sessions) {
    if (session.scope !== 'project') continue
    groups.get(session.workspacePath)?.sessions.push(session)
  }
  return {
    chatSessions: pinnedFirst(
      sessions.filter((session) => session.scope === 'chat'),
      pinned,
    ),
    workspaceGroups: [...groups.values()].map((group) => ({
      ...group,
      sessions: pinnedFirst(group.sessions, pinned),
    })),
  }
}

export function resizedSidebarWidth(
  startWidth: number,
  startX: number,
  currentX: number,
) {
  return clampSidebarWidth(startWidth + currentX - startX)
}

export function keyboardSidebarWidth(
  key: string,
  currentWidth: number,
): number | undefined {
  let nextWidth: number | undefined
  if (key === 'ArrowLeft') nextWidth = currentWidth - 8
  if (key === 'ArrowRight') nextWidth = currentWidth + 8
  if (key === 'Home') nextWidth = MIN_SIDEBAR_WIDTH
  if (key === 'End') nextWidth = MAX_SIDEBAR_WIDTH
  return nextWidth === undefined ? undefined : clampSidebarWidth(nextWidth)
}
