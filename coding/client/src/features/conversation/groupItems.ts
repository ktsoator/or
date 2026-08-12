import type { Item } from '@/types'

export type RenderUnit =
  | { kind: 'item'; item: Item }
  | { kind: 'steps'; id: string; items: Item[] }

export type ConversationUnit =
  | RenderUnit
  | {
      kind: 'assistant-turn'
      id: string
      messageID?: string
      units: RenderUnit[]
    }

// groupItems folds each maximal run of consecutive tool/thinking items into a
// step group when it holds two or more tool calls. A tool whose input is still
// streaming stays inline so its live progress is never hidden by a newly formed
// collapsed group; it joins the group once execution starts.
export function groupItems(items: Item[]): RenderUnit[] {
  const units: RenderUnit[] = []
  let buffer: Item[] = []

  const flush = () => {
    if (buffer.length === 0) return
    const toolCount = buffer.filter((it) => it.kind === 'tool').length
    if (toolCount >= 2) {
      units.push({ kind: 'steps', id: `steps-${buffer[0].id}`, items: buffer })
    } else {
      for (const it of buffer) units.push({ kind: 'item', item: it })
    }
    buffer = []
  }

  for (const item of items) {
    if (item.kind === 'tool' && item.status === 'preparing') {
      flush()
      units.push({ kind: 'item', item })
      continue
    }
    if (item.kind === 'tool' || item.kind === 'thinking') {
      buffer.push(item)
    } else {
      flush()
      units.push({ kind: 'item', item })
    }
  }
  flush()
  return units
}

// groupAssistantTurns keeps the run status and any immediately following
// thinking/tool activity with the final response they led to. A user message or
// an earlier assistant response always starts a new boundary.
export function groupAssistantTurns(items: Item[]): ConversationUnit[] {
  const units = groupItems(items)
  const grouped: ConversationUnit[] = []

  for (const unit of units) {
    if (unit.kind !== 'item' || unit.item.kind !== 'assistant') {
      grouped.push(unit)
      continue
    }

    let start = grouped.length
    while (start > 0 && isAssistantLeadUnit(grouped[start - 1])) start -= 1

    const leadUnits = grouped.splice(start) as RenderUnit[]
    grouped.push({
      kind: 'assistant-turn',
      id: `assistant-turn-${unit.item.id}`,
      messageID: unit.item.messageID,
      units: [...leadUnits, unit],
    })
  }

  return grouped
}

function isAssistantLeadUnit(unit: ConversationUnit): unit is RenderUnit {
  if (unit.kind === 'steps') return true
  if (unit.kind !== 'item') return false
  return unit.item.kind === 'run' || unit.item.kind === 'thinking' || unit.item.kind === 'tool'
}
