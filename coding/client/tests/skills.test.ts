import { describe, expect, test } from 'bun:test'
import {
  buildSkillInvocation,
  filterSkills,
  parseSkillSlashQuery,
  skillArgumentsFromDraft,
  type SkillEntry,
} from '../src/skills'

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
    expect(buildSkillInvocation('pptx', 'make slides')).toBe('/skill:pptx make slides')
    expect(buildSkillInvocation('review', '  ')).toBe('/skill:review')
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
