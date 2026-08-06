import { describe, expect, test } from 'bun:test'
import {
  containsHighlightableCode,
  highlightCode,
  highlightLanguage,
  languageForPath,
  type SyntaxHighlighter,
} from '../src/shared/lib/highlight'

const highlighter: SyntaxHighlighter = {
  getLanguage: (name) => (name === 'typescript' ? {} : undefined),
  highlight: (code, { language }) => ({ value: `${language}:${code}` }),
}

describe('syntax highlighting helpers', () => {
  test('maps common file extensions without loading the highlighter runtime', () => {
    expect(languageForPath('/work/Component.TSX')).toBe('typescript')
    expect(languageForPath('/work/config.yml')).toBe('yaml')
    expect(languageForPath('/work/README')).toBe('plaintext')
  })

  test('escapes code while the asynchronous highlighter is unavailable', () => {
    expect(highlightCode('<tag title="a&b">', 'xml')).toBe(
      '&lt;tag title=&quot;a&amp;b&quot;&gt;',
    )
  })

  test('uses loaded languages and falls back for unknown ones', () => {
    expect(highlightLanguage(' TypeScript extra', highlighter)).toBe('typescript')
    expect(highlightLanguage('unknown', highlighter)).toBe('plaintext')
    expect(highlightCode('const value = 1', 'typescript', highlighter)).toBe(
      'typescript:const value = 1',
    )
  })

  test('detects block code without treating inline code as a loading signal', () => {
    expect(containsHighlightableCode('before\n```ts\nconst value = 1\n```')).toBe(true)
    expect(containsHighlightableCode('before\n~~~js\nvalue()\n~~~')).toBe(true)
    expect(containsHighlightableCode('before\n    indented code')).toBe(true)
    expect(containsHighlightableCode('Use `inlineCode()` here.')).toBe(false)
    expect(containsHighlightableCode('Ordinary prose only.')).toBe(false)
  })
})
