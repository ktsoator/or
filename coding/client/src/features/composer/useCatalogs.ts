import { useEffect, useMemo, useState } from 'react'
import type { Locale } from '@/i18n'
import {
  fetchPromptTemplates,
  localizePromptTemplate,
  type PromptTemplatesResponse,
} from '@/features/prompt-templates'
import { fetchSkills, type SkillsResponse } from '@/features/skills'

export function useComposerCatalogs({
  workspacePath,
  locale,
  catalogOpen,
}: {
  workspacePath?: string
  locale: Locale
  catalogOpen: boolean
}) {
  const [skillsData, setSkillsData] = useState<SkillsResponse>()
  const [skillsLoading, setSkillsLoading] = useState(false)
  const [skillsFailed, setSkillsFailed] = useState(false)
  const [promptTemplatesData, setPromptTemplatesData] =
    useState<PromptTemplatesResponse>()
  const [promptTemplatesLoading, setPromptTemplatesLoading] = useState(false)
  const [promptTemplatesFailed, setPromptTemplatesFailed] = useState(false)

  useEffect(() => {
    setSkillsData(undefined)
    setSkillsFailed(false)
    setPromptTemplatesData(undefined)
    setPromptTemplatesFailed(false)
  }, [workspacePath])

  useEffect(() => {
    if (!catalogOpen) return
    const controller = new AbortController()
    setSkillsLoading(true)
    setSkillsFailed(false)
    setSkillsData(undefined)
    void fetchSkills(workspacePath, controller.signal)
      .then(setSkillsData)
      .catch((error) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setSkillsFailed(true)
      })
      .finally(() => {
        if (!controller.signal.aborted) setSkillsLoading(false)
      })
    return () => controller.abort()
  }, [catalogOpen, workspacePath])

  useEffect(() => {
    if (!catalogOpen) return
    const controller = new AbortController()
    setPromptTemplatesLoading(true)
    setPromptTemplatesFailed(false)
    setPromptTemplatesData(undefined)
    void fetchPromptTemplates(workspacePath, controller.signal)
      .then(setPromptTemplatesData)
      .catch((error) => {
        if (error instanceof DOMException && error.name === 'AbortError') return
        setPromptTemplatesFailed(true)
      })
      .finally(() => {
        if (!controller.signal.aborted) setPromptTemplatesLoading(false)
      })
    return () => controller.abort()
  }, [catalogOpen, workspacePath])

  const skills = useMemo(
    () => [...(skillsData?.project ?? []), ...(skillsData?.user ?? [])],
    [skillsData],
  )
  const promptTemplates = useMemo(
    () => [
      ...(promptTemplatesData?.project ?? []),
      ...(promptTemplatesData?.user ?? []),
    ].map((template) => localizePromptTemplate(template, locale)),
    [locale, promptTemplatesData],
  )

  return {
    skills,
    promptTemplates,
    skillsLoaded: Boolean(skillsData),
    promptTemplatesLoaded: Boolean(promptTemplatesData),
    skillsLoading,
    skillsFailed,
    promptTemplatesLoading,
    promptTemplatesFailed,
  }
}
