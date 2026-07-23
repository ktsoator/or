import type { Item } from '@/types'

export type RenderUnit =
  | { kind: 'item'; item: Item }
  | { kind: 'steps'; id: string; items: Item[] }

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
