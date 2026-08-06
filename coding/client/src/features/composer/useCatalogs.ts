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
  refreshPromptTemplates,
}: {
  workspacePath?: string
  locale: Locale
  refreshPromptTemplates: boolean
}) {
  const [skillsData, setSkillsData] = useState<SkillsResponse>()
  const [skillsLoading, setSkillsLoading] = useState(true)
  const [skillsFailed, setSkillsFailed] = useState(false)
  const [promptTemplatesData, setPromptTemplatesData] =
    useState<PromptTemplatesResponse>()
  const [promptTemplatesLoading, setPromptTemplatesLoading] = useState(true)
  const [promptTemplatesFailed, setPromptTemplatesFailed] = useState(false)

  useEffect(() => {
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
  }, [workspacePath])

  useEffect(() => {
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
  }, [refreshPromptTemplates, workspacePath])

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
