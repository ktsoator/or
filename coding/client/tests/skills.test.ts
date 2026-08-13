import { describe, expect, test } from 'bun:test'
import {
  buildSkillInvocation,
  filterSkills,
  parseSkillReference,
  serializeSkillReferenceCopy,
  type SkillEntry,
} from '../src/features/skills'
import {
  composerPreviewCommands,
  moveSuggestionIndex,
  parseExecutableComposerCommand,
  previewSkillCommandCount,
} from '../src/features/composer/panelStyles'

describe('Skill composer mentions', () => {
  test('builds the backend command only at submission time', () => {
    expect(
      buildSkillInvocation(
        { name: 'pptx', dir: '/skills/pptx', path: '/skills/pptx/SKILL.md' },
        'make slides',
      ),
    ).toBe('[$pptx](/skills/pptx/SKILL.md) make slides')
    expect(
      buildSkillInvocation({ name: 'review', dir: '/skills/review path' }, '  '),
    ).toBe('[$review](</skills/review path/SKILL.md>)')
  })

  test('parses and serializes rich skill references', () => {
    const reference = parseSkillReference(
      '[$frontend-design](</skills/frontend design/SKILL.md>) 做一个网页',
    )
    expect(reference).toEqual({
      name: 'frontend-design',
      path: '/skills/frontend design/SKILL.md',
      argumentsText: '做一个网页',
      markdown: '[$frontend-design](</skills/frontend design/SKILL.md>)',
    })
    expect(serializeSkillReferenceCopy(reference!, 'frontend-design 做一个')).toBe(
      '[$frontend-design](</skills/frontend design/SKILL.md>) 做一个',
    )
    expect(serializeSkillReferenceCopy(reference!, '做一个')).toBe('做一个')
  })
})

describe('filterSkills', () => {
  const skills: SkillEntry[] = [
    {
      name: 'pdf',
      description: 'Read and create PDF documents',
      source: 'user',
      dir: '/skills/pdf',
    },
    {
      name: 'frontend-design',
      description: 'Build polished interfaces',
      source: 'project',
      dir: '/skills/frontend-design',
    },
  ]

  test('matches names and descriptions without changing catalog order', () => {
    expect(filterSkills(skills, 'PDF')).toEqual([skills[0]])
    expect(filterSkills(skills, 'polished')).toEqual([skills[1]])
    expect(filterSkills(skills, '')).toEqual(skills)
  })
})

describe('Composer preview commands', () => {
  test('keeps compact at a stable keyboard index', () => {
    expect(composerPreviewCommands[1]).toBe('compact')
    expect(previewSkillCommandCount('')).toBe(composerPreviewCommands.length)
    expect(previewSkillCommandCount('compact')).toBe(0)
  })

  test('wraps keyboard navigation across both catalog boundaries', () => {
    expect(moveSuggestionIndex(0, 12, 'previous')).toBe(11)
    expect(moveSuggestionIndex(11, 12, 'next')).toBe(0)
    expect(moveSuggestionIndex(5, 12, 'previous')).toBe(4)
    expect(moveSuggestionIndex(5, 12, 'next')).toBe(6)
  })

  test('recognizes only the executable compact command', () => {
    expect(parseExecutableComposerCommand('/compact')).toBe('compact')
    expect(parseExecutableComposerCommand('  /compact  ')).toBe('compact')
    expect(parseExecutableComposerCommand('/compact now')).toBeUndefined()
    expect(parseExecutableComposerCommand('/review')).toBeUndefined()
  })
})
