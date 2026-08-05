import { describe, expect, test } from 'bun:test'
import {
  buildSkillInvocation,
  displaySkillInvocation,
  filterSkills,
  parseSkillSlashQuery,
  parseSkillReference,
  serializeSkillReferenceCopy,
  skillArgumentsFromDraft,
  type SkillEntry,
} from '../src/skills'
import {
  composerPreviewCommands,
  parseExecutableComposerCommand,
  previewSkillCommandCount,
} from '../src/components/composerPanelStyles'

describe('Skill composer commands', () => {
  test('recognizes slash search and preserves arguments', () => {
    expect(parseSkillSlashQuery('/pdf 类似这种的')).toEqual({
      query: 'pdf',
      argumentsText: '类似这种的',
    })
    expect(parseSkillSlashQuery('/')).toEqual({ query: '', argumentsText: '' })
    expect(parseSkillSlashQuery('/skill:pdf existing')).toBeUndefined()
  })

  test('extracts arguments when replacing slash or explicit commands', () => {
    expect(skillArgumentsFromDraft('/pdf make slides')).toBe('make slides')
    expect(skillArgumentsFromDraft('/skill:old make slides')).toBe('make slides')
    expect(skillArgumentsFromDraft('make slides')).toBe('make slides')
  })

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

  test('upgrades legacy backend commands into skill references', () => {
    expect(displaySkillInvocation('/skill:pptx make slides')).toBe('[$pptx]() make slides')
    expect(displaySkillInvocation('/skill:review')).toBe('[$review]()')
    expect(displaySkillInvocation('/review existing')).toBe('/review existing')
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
      disableModelInvocation: false,
    },
    {
      name: 'frontend-design',
      description: 'Build polished interfaces',
      source: 'project',
      dir: '/skills/frontend-design',
      disableModelInvocation: false,
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

  test('recognizes only the executable compact command', () => {
    expect(parseExecutableComposerCommand('/compact')).toBe('compact')
    expect(parseExecutableComposerCommand('  /compact  ')).toBe('compact')
    expect(parseExecutableComposerCommand('/compact now')).toBeUndefined()
    expect(parseExecutableComposerCommand('/review')).toBeUndefined()
  })
})
