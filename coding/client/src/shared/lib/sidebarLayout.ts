export const DEFAULT_SIDEBAR_WIDTH = 240
export const MIN_SIDEBAR_WIDTH = 206
export const MAX_SIDEBAR_WIDTH = 338

export function clampSidebarWidth(width: number) {
  return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, width))
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
