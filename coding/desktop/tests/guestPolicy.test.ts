import { describe, expect, test } from 'bun:test'
import {
  browserPartition,
  guestKindForPartition,
  popupNavigationAction,
  previewNavigationAction,
  previewPartition,
} from '../src/guestPolicy'

describe('guest policy', () => {
  test('recognizes only the two browser partitions', () => {
    expect(guestKindForPartition(previewPartition)).toBe('preview')
    expect(guestKindForPartition(browserPartition)).toBe('web')
    expect(guestKindForPartition('')).toBeUndefined()
    expect(guestKindForPartition('persist:default')).toBeUndefined()
  })

  test('keeps preview navigation on its exact origin', () => {
    const origin = 'http://127.0.0.1:41000'
    expect(previewNavigationAction(`${origin}/assets/app.js`, origin)).toBe('allow')
    expect(previewNavigationAction('http://127.0.0.1:42000/', origin)).toBe('open-tab')
    expect(previewNavigationAction('https://example.com/docs', origin)).toBe('open-tab')
  })

  test('routes external protocols without allowing unsafe popups', () => {
    expect(popupNavigationAction('https://example.com')).toBe('open-tab')
    expect(popupNavigationAction('mailto:team@example.com')).toBe('open-external')
    expect(popupNavigationAction('file:///tmp/private')).toBe('deny')
    expect(popupNavigationAction('javascript:alert(1)')).toBe('deny')
  })
})
