import { describe, expect, test } from 'bun:test'
import {
  clampWorkbenchWidth,
  isWorkbenchConstrained,
  keyboardWorkbenchWidth,
  resizedWorkbenchWidth,
  workbenchWidthBounds,
} from '../src/workbenchLayout'

describe('workbench layout', () => {
  test('keeps the workbench and Chat within their width constraints', () => {
    expect(workbenchWidthBounds(1200).minimum).toBe(300)
    expect(workbenchWidthBounds(1200).maximum).toBeCloseTo(816)
    expect(workbenchWidthBounds(800)).toEqual({ minimum: 300, maximum: 440 })
    expect(workbenchWidthBounds(500)).toEqual({ minimum: 300, maximum: 300 })
    expect(workbenchWidthBounds(240)).toEqual({ minimum: 240, maximum: 240 })

    expect(clampWorkbenchWidth(200, 1200)).toBe(300)
    expect(clampWorkbenchWidth(500, 1200)).toBe(500)
    expect(clampWorkbenchWidth(900, 1200)).toBeCloseTo(816)
  })

  test('uses hysteresis when entering and leaving constrained mode', () => {
    expect(isWorkbenchConstrained(800, 280, false)).toBe(false)
    expect(isWorkbenchConstrained(800, 281, false)).toBe(true)

    expect(isWorkbenchConstrained(800, 240, true)).toBe(false)
    expect(isWorkbenchConstrained(800, 241, true)).toBe(true)
  })

  test('resizes from the left edge and clamps pointer movement', () => {
    expect(resizedWorkbenchWidth(500, 1000, 900, 1200)).toBe(600)
    expect(resizedWorkbenchWidth(500, 1000, 1200, 1200)).toBe(300)
    expect(resizedWorkbenchWidth(500, 1000, 500, 1200)).toBeCloseTo(816)
  })

  test('maps keyboard controls to bounded workbench widths', () => {
    expect(keyboardWorkbenchWidth('ArrowLeft', 500, 1200)).toBe(516)
    expect(keyboardWorkbenchWidth('ArrowRight', 500, 1200)).toBe(484)
    expect(keyboardWorkbenchWidth('Home', 500, 1200)).toBe(300)
    expect(keyboardWorkbenchWidth('End', 500, 1200)).toBeCloseTo(816)
    expect(keyboardWorkbenchWidth('Escape', 500, 1200)).toBeUndefined()
  })
})
