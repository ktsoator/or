import { describe, expect, test } from 'bun:test'
import {
  buildPromptTemplateInvocation,
  filterPromptTemplates,
  localizePromptTemplate,
  parsePromptTemplateInvocation,
  parsePromptTemplateQuery,
  promptTemplateArgumentsText,
  serializePromptTemplateInvocationCopy,
  type PromptTemplateEntry,
} from '../src/features/prompt-templates'

describe('Prompt template composer commands', () => {
  const templates: PromptTemplateEntry[] = [
    {
      name: 'review',
      description: 'Review working tree changes',
      argumentHint: '[focus]',
      source: 'project',
      path: '/workspace/.or/prompts/review.md',
    },
    {
      name: 'commit',
      description: 'Write a commit message',
      argumentHint: '',
      source: 'user',
      path: '/home/user/.or/prompts/commit.md',
    },
  ]

  test('filters by name and description without changing order', () => {
    expect(filterPromptTemplates(templates, 'REVIEW')).toEqual([templates[0]])
    expect(filterPromptTemplates(templates, 'commit message')).toEqual([
      templates[1],
    ])
    expect(filterPromptTemplates(templates, '')).toEqual(templates)
  })

  test('builds a compact visible invocation', () => {
    expect(buildPromptTemplateInvocation('review', 'security')).toBe(
      '/review security',
    )
    expect(buildPromptTemplateInvocation('review', '  ')).toBe('/review')
  })

  test('keeps slash template search separate from dollar skill search', () => {
    expect(parsePromptTemplateQuery('/review security')).toEqual({
      query: 'review',
      argumentsText: 'security',
    })
    expect(parsePromptTemplateQuery('/')).toEqual({ query: '', argumentsText: '' })
    expect(parsePromptTemplateQuery('$review security')).toBeUndefined()
  })

  test('parses a visible invocation into its token and arguments', () => {
    expect(parsePromptTemplateInvocation('/review security')).toEqual({
      name: 'review',
      argumentsText: 'security',
    })
    expect(parsePromptTemplateInvocation('/review')).toEqual({
      name: 'review',
      argumentsText: '',
    })
    expect(parsePromptTemplateInvocation('review')).toBeUndefined()
    expect(parsePromptTemplateInvocation('/usr/local/bin')).toBeUndefined()
  })

  test('extracts arguments only for the backend-resolved template name', () => {
    expect(promptTemplateArgumentsText('/review security', 'review')).toBe('security')
    expect(promptTemplateArgumentsText('/reviewer security', 'review')).toBe('')
    expect(promptTemplateArgumentsText('/hello', 'review')).toBe('')
  })

  test('copies a display label as its stored slash invocation', () => {
    const invocation = parsePromptTemplateInvocation('/review security')!
    expect(serializePromptTemplateInvocationCopy(invocation, 'review')).toBe(
      '/review',
    )
    expect(serializePromptTemplateInvocationCopy(invocation, 'ordinary text')).toBe(
      'ordinary text',
    )
  })

  test('uses localized metadata with legacy field fallback', () => {
    const localized = {
      ...templates[0],
      description: '旧描述',
      descriptions: {
        en: 'Review working tree changes',
        'zh-CN': '审查当前代码改动',
      },
      argumentHint: '[旧参数]',
      argumentHints: { en: '[focus]', 'zh-CN': '[关注点]' },
    }
    expect(localizePromptTemplate(localized, 'en')).toMatchObject({
      description: 'Review working tree changes',
      argumentHint: '[focus]',
    })
    expect(localizePromptTemplate(localized, 'zh-CN')).toMatchObject({
      description: '审查当前代码改动',
      argumentHint: '[关注点]',
    })
    expect(localizePromptTemplate(templates[1], 'zh-CN')).toBe(templates[1])
  })
})
