import type { SessionSummary, WorkspaceSummary } from '@/types'

export type WorkspaceSessionGroup = {
  path: string
  name: string
  sessions: SessionSummary[]
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
