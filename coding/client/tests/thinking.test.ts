import { describe, expect, test } from 'bun:test'
import { parseThinkingContent } from '../src/features/conversation/thinkingContent'

describe('parseThinkingContent', () => {
  test('promotes a complete leading bold line and removes it from the body', () => {
    expect(
      parseThinkingContent(
        '**Considering workspace modifications**\n\nI need to inspect the current files.',
      ),
    ).toEqual({
      title: 'Considering workspace modifications',
      body: 'I need to inspect the current files.',
    })
  })

  test('keeps an incomplete streaming title in the body', () => {
    expect(parseThinkingContent('**Considering workspace modifi')).toEqual({
      body: '**Considering workspace modifi',
    })
  })

  test('leaves ordinary reasoning text unchanged', () => {
    expect(parseThinkingContent('I need to inspect the current files.')).toEqual({
      body: 'I need to inspect the current files.',
    })
  })

  test('supports a title without a body', () => {
    expect(parseThinkingContent('**Evaluating implementation**')).toEqual({
      title: 'Evaluating implementation',
      body: '',
    })
  })

  test('does not promote bold text followed by content on the same line', () => {
    const text = '**Important** but this is still body text.'
    expect(parseThinkingContent(text)).toEqual({ body: text })
  })
})
