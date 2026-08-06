export const DEFAULT_WORKBENCH_RATIO = 0.48
export const MIN_WORKBENCH_WIDTH = 300
export const MIN_CHAT_WIDTH = 360
export const MAX_WORKBENCH_RATIO = 0.68
export const AUTO_COLLAPSE_CHAT_WIDTH = 520
export const AUTO_COLLAPSE_RESET_WIDTH = 560

export function workbenchWidthBounds(layoutWidth: number) {
  const minimum = Math.min(MIN_WORKBENCH_WIDTH, layoutWidth)
  const maximum = Math.max(
    minimum,
    Math.min(layoutWidth * MAX_WORKBENCH_RATIO, layoutWidth - MIN_CHAT_WIDTH),
  )
  return { minimum, maximum }
}

export function clampWorkbenchWidth(width: number, layoutWidth: number) {
  const { minimum, maximum } = workbenchWidthBounds(layoutWidth)
  return Math.min(maximum, Math.max(minimum, width))
}

export function isWorkbenchConstrained(
  layoutWidth: number,
  workbenchWidth: number,
  wasConstrained: boolean,
) {
  const availableChatWidth = layoutWidth - workbenchWidth
  return wasConstrained
    ? availableChatWidth < AUTO_COLLAPSE_RESET_WIDTH
    : availableChatWidth < AUTO_COLLAPSE_CHAT_WIDTH
}

export function resizedWorkbenchWidth(
  startWidth: number,
  startX: number,
  currentX: number,
  layoutWidth: number,
) {
  return clampWorkbenchWidth(startWidth + startX - currentX, layoutWidth)
}

export function keyboardWorkbenchWidth(
  key: string,
  currentWidth: number,
  layoutWidth: number,
): number | undefined {
  const { minimum, maximum } = workbenchWidthBounds(layoutWidth)
  let nextWidth: number | undefined
  if (key === 'ArrowLeft') nextWidth = currentWidth + 16
  if (key === 'ArrowRight') nextWidth = currentWidth - 16
  if (key === 'Home') nextWidth = minimum
  if (key === 'End') nextWidth = maximum
  return nextWidth === undefined ? undefined : clampWorkbenchWidth(nextWidth, layoutWidth)
}
