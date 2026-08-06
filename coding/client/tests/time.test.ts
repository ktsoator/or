import { describe, expect, test } from 'bun:test'
import { formatMessageTime } from '../src/lib/time'

describe('formatMessageTime', () => {
  const now = new Date(2026, 7, 5, 15, 30)

  test('shows only the time for messages from today', () => {
    const message = new Date(2026, 7, 5, 10, 52)

    expect(formatMessageTime(message.toISOString(), 'en', now)).toBe('10:52 AM')
    expect(formatMessageTime(message.toISOString(), 'zh-CN', now)).toBe('10:52')
  })

  test('adds the date for earlier messages and the year when needed', () => {
    const earlier = new Date(2026, 7, 4, 10, 52)
    const previousYear = new Date(2025, 7, 4, 10, 52)
    const earlierEnglish = formatMessageTime(earlier.toISOString(), 'en', now)
    const previousYearEnglish = formatMessageTime(previousYear.toISOString(), 'en', now)

    expect(earlierEnglish).toContain('Aug 4')
    expect(earlierEnglish).toContain('10:52 AM')
    expect(earlierEnglish).not.toMatch(/\b20\d{2}\b/)
    expect(formatMessageTime(earlier.toISOString(), 'zh-CN', now)).toBe('8月4日 10:52')
    expect(previousYearEnglish).toContain('Aug 4')
    expect(previousYearEnglish).toContain('2025')
    expect(previousYearEnglish).toContain('10:52 AM')
    expect(formatMessageTime(previousYear.toISOString(), 'zh-CN', now)).toBe(
      '2025年8月4日 10:52',
    )
  })
})
