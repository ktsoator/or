import { useEffect, useMemo, useState } from 'react'
import { fetchSkills, type SkillsResponse } from '@/features/skills'

export function useComposerCatalogs({
  workspacePath,
  catalogOpen,
}: {
  workspacePath?: string
  catalogOpen: boolean
}) {
  const [skillsData, setSkillsData] = useState<SkillsResponse>()
  const [skillsLoading, setSkillsLoading] = useState(false)
  const [skillsFailed, setSkillsFailed] = useState(false)

  useEffect(() => {
    setSkillsData(undefined)
    setSkillsFailed(false)
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

  const skills = useMemo(
    () => [...(skillsData?.project ?? []), ...(skillsData?.user ?? [])],
    [skillsData],
  )
  return {
    skills,
    skillsLoaded: Boolean(skillsData),
    skillsLoading,
    skillsFailed,
  }
}
