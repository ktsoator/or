import { useCallback, useEffect, useState } from 'react'
import { ArrowLeft, LoaderCircle, X } from 'lucide-react'
import { apiURL } from '@/api'
import { useI18n } from '@/i18n'
import { Markdown } from './Markdown'
import { SidebarToggleButton } from './SidebarToggleButton'

type SkillEntry = {
  name: string
  description: string
  source: 'user' | 'project'
  dir: string
}

type SkillDiagnostic = {
  path: string
  message: string
}

type SkillsResponse = {
  user: SkillEntry[]
  project: SkillEntry[]
  diagnostics: SkillDiagnostic[]
}

// relativizeHome trims the user's home prefix off an absolute path so directory
// hints read as "~/.or/skills/commit" rather than a long absolute path.
function relativizeHome(path: string): string {
  const home = path.match(/^(\/(?:Users|home)\/[^/]+)/)?.[1]
  return home ? path.replace(home, '~') : path
}

export function SkillsPage({
  onBack,
  sidebarCollapsed,
  onExpandSidebar,
  workspacePath,
  workspaceName,
}: {
  onBack: () => void
  sidebarCollapsed?: boolean
  onExpandSidebar?: () => void
  workspacePath?: string
  workspaceName?: string
}) {
  const { t } = useI18n()
  const [data, setData] = useState<SkillsResponse>()
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<SkillEntry>()

  const load = useCallback(async () => {
    setLoading(true)
    setError(false)
    try {
      const query = workspacePath ? `?workspace=${encodeURIComponent(workspacePath)}` : ''
      const response = await fetch(apiURL(`/skills${query}`), { cache: 'no-store' })
      if (!response.ok) throw new Error('failed')
      setData((await response.json()) as SkillsResponse)
    } catch {
      setError(true)
    } finally {
      setLoading(false)
    }
  }, [workspacePath])

  useEffect(() => {
    void load()
  }, [load])

  const total = (data?.project.length ?? 0) + (data?.user.length ?? 0)

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-white">
      <header
        className={`skills-header window-titlebar z-20 flex h-[45px] shrink-0 items-center gap-1 border-b border-stone-200/80 bg-white px-4 max-md:h-12 ${sidebarCollapsed ? 'sidebar-is-collapsed' : ''}`}
      >
        {sidebarCollapsed && onExpandSidebar && (
          <SidebarToggleButton
            expanded={false}
            className="desktop-sidebar-toggle hidden md:grid"
            onToggle={onExpandSidebar}
          />
        )}
        <button
          className="window-titlebar-control flex h-9 cursor-pointer items-center gap-2 rounded-[10px] px-2.5 text-[0.84375rem] font-normal text-stone-500 outline-none transition-colors hover:bg-stone-200/65 hover:text-stone-900 focus-visible:ring-2 focus-visible:ring-stone-300"
          type="button"
          onClick={onBack}
        >
          <ArrowLeft className="size-4" aria-hidden="true" />
          <span>{t('skills.back')}</span>
        </button>
      </header>

      <main className="min-h-0 flex-1 overflow-y-auto bg-white">
        <div className="mx-auto w-full max-w-[56rem] px-10 pt-12 pb-24 max-lg:px-7 max-md:px-4 max-md:pt-8">
          <div className="flex items-center gap-2.5">
            <h1 className="text-[1.75rem] leading-9 font-semibold tracking-[-0.035em] text-stone-950 max-md:text-[1.5rem]">
              {t('skills.title')}
            </h1>
            {!loading && !error && total > 0 && (
              <span className="mt-1 rounded-full bg-stone-100 px-2 py-0.5 text-[0.6875rem] font-medium text-stone-500">
                {total}
              </span>
            )}
          </div>
          <p className="mt-1 max-w-[34rem] text-[0.84375rem] leading-5 text-stone-500">
            {t('skills.subtitle')}
          </p>

          {loading ? (
            <div className="mt-16 flex items-center justify-center gap-2 text-[0.84375rem] text-stone-400">
              <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              {t('skills.loading')}
            </div>
          ) : error ? (
            <div className="mt-12 flex flex-col items-center gap-3 rounded-[20px] border border-stone-200/90 bg-white px-8 py-14 text-center shadow-[0_10px_32px_-30px_rgba(28,25,23,0.45)]">
              <p className="text-[0.84375rem] text-stone-500">{t('skills.loadError')}</p>
              <button
                className="h-8 cursor-pointer rounded-[10px] border border-stone-200 bg-white px-3.5 text-[0.8125rem] text-stone-700 transition-colors hover:bg-stone-100"
                type="button"
                onClick={() => void load()}
              >
                {t('skills.retry')}
              </button>
            </div>
          ) : (
            <div className="mt-10 space-y-9">
              <SkillSection
                title={workspaceName ? t('skills.projectSectionNamed', { name: workspaceName }) : t('skills.projectSection')}
                hint={workspacePath ? relativizeHome(`${workspacePath}/.or/skills`) : t('skills.noProject')}
                skills={data?.project ?? []}
                empty={workspacePath ? t('skills.emptyProject') : t('skills.noProjectHint')}
                onSelect={setSelected}
              />
              <SkillSection
                title={t('skills.systemSection')}
                hint="~/.or/skills"
                skills={data?.user ?? []}
                empty={t('skills.emptySystem')}
                onSelect={setSelected}
              />
              {data && data.diagnostics.length > 0 && (
                <SkillDiagnostics diagnostics={data.diagnostics} />
              )}
            </div>
          )}
        </div>
      </main>

      {selected && (
        <SkillDetailDialog
          skill={selected}
          workspacePath={workspacePath}
          onClose={() => setSelected(undefined)}
        />
      )}
    </div>
  )
}

function SkillSection({
  title,
  hint,
  skills,
  empty,
  onSelect,
}: {
  title: string
  hint: string
  skills: SkillEntry[]
  empty: string
  onSelect: (skill: SkillEntry) => void
}) {
  return (
    <section>
      <div className="mb-3.5 flex items-center justify-between gap-4">
        <div className="flex shrink-0 items-center gap-2">
          <h2 className="text-[0.9375rem] leading-5 font-medium text-stone-900">{title}</h2>
          <span className="grid h-[1.15rem] min-w-[1.15rem] shrink-0 place-items-center rounded-full bg-stone-100 px-1.5 text-[0.6875rem] font-medium text-stone-500">
            {skills.length}
          </span>
        </div>
        <span className="min-w-0 flex-1 truncate text-right font-mono text-[0.71875rem] text-stone-400 max-sm:hidden" title={hint}>
          {hint}
        </span>
      </div>
      {skills.length === 0 ? (
        <div className="rounded-[20px] border border-dashed border-stone-200 bg-stone-50/50 px-6 py-11 text-center">
          <p className="mx-auto max-w-[22rem] text-[0.8125rem] leading-5 text-stone-400">{empty}</p>
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-2.5 max-md:grid-cols-1">
          {skills.map((skill) => (
            <SkillCard key={`${skill.source}-${skill.name}`} skill={skill} onSelect={onSelect} />
          ))}
        </div>
      )}
    </section>
  )
}

function SkillCard({ skill, onSelect }: { skill: SkillEntry; onSelect: (skill: SkillEntry) => void }) {
  return (
    <button
      type="button"
      className="flex cursor-pointer flex-col gap-0.5 rounded-[18px] border border-stone-200 bg-white px-4 py-3.5 text-left transition-colors outline-none hover:border-stone-300 hover:bg-stone-50/60 focus-visible:border-stone-400 focus-visible:ring-2 focus-visible:ring-stone-200"
      title={relativizeHome(skill.dir)}
      onClick={() => onSelect(skill)}
    >
      <div className="truncate font-mono text-[0.84375rem] font-medium text-stone-900">{skill.name}</div>
      <p className="line-clamp-2 text-[0.8125rem] leading-[1.45] text-stone-500">{skill.description}</p>
    </button>
  )
}

type SkillDetail = SkillEntry & { content: string }

function SkillDetailDialog({
  skill,
  workspacePath,
  onClose,
}: {
  skill: SkillEntry
  workspacePath?: string
  onClose: () => void
}) {
  const { t } = useI18n()
  const [detail, setDetail] = useState<SkillDetail>()
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [onClose])

  useEffect(() => {
    let active = true
    setLoading(true)
    setError(false)
    const query = workspacePath ? `?workspace=${encodeURIComponent(workspacePath)}` : ''
    fetch(apiURL(`/skills/${encodeURIComponent(skill.name)}${query}`), { cache: 'no-store' })
      .then((response) => {
        if (!response.ok) throw new Error('failed')
        return response.json() as Promise<SkillDetail>
      })
      .then((body) => {
        if (active) setDetail(body)
      })
      .catch(() => {
        if (active) setError(true)
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [skill.name, workspacePath])

  const sourceLabel = skill.source === 'project' ? t('skills.systemSourceProject') : t('skills.systemSourceUser')

  return (
    <div
      className="fixed inset-0 z-[140] grid place-items-center bg-stone-950/20 px-4"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <section
        className="flex max-h-[min(85vh,48rem)] w-full max-w-[44rem] flex-col overflow-hidden rounded-[16px] border border-stone-300/80 bg-white shadow-[0_24px_64px_-28px_rgba(28,25,23,0.42)] animate-[fade-in_100ms_ease-out]"
        role="dialog"
        aria-modal="true"
        aria-label={skill.name}
      >
        <header className="flex items-start gap-3 border-b border-stone-200/70 px-5 py-4">
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h2 className="truncate font-mono text-[0.9375rem] font-semibold text-stone-950">{skill.name}</h2>
              <span className="shrink-0 rounded-full bg-stone-100 px-2 py-0.5 text-[0.6875rem] font-medium text-stone-500">
                {sourceLabel}
              </span>
            </div>
            <p className="mt-1 text-[0.8125rem] leading-5 text-stone-500">{skill.description}</p>
            <p className="mt-1 truncate font-mono text-[0.6875rem] text-stone-400" title={skill.dir}>
              {relativizeHome(skill.dir)}
            </p>
          </div>
          <button
            className="-mt-0.5 grid size-7 shrink-0 cursor-pointer place-items-center rounded-[8px] text-stone-400 transition-colors hover:bg-[rgb(246,246,246)] hover:text-stone-800"
            type="button"
            aria-label={t('skills.close')}
            onClick={onClose}
          >
            <X className="size-3.5" aria-hidden="true" />
          </button>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {loading ? (
            <div className="flex items-center justify-center gap-2 py-10 text-[0.84375rem] text-stone-400">
              <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              {t('skills.loading')}
            </div>
          ) : error ? (
            <p className="py-10 text-center text-[0.84375rem] text-stone-500">{t('skills.loadError')}</p>
          ) : detail && detail.content.trim() ? (
            <div className="text-[0.875rem] leading-relaxed text-stone-800">
              <Markdown source={detail.content} />
            </div>
          ) : (
            <p className="py-10 text-center text-[0.84375rem] text-stone-400">{t('skills.emptyBody')}</p>
          )}
        </div>
      </section>
    </div>
  )
}

function SkillDiagnostics({ diagnostics }: { diagnostics: SkillDiagnostic[] }) {
  const { t } = useI18n()
  return (
    <section>
      <div className="mb-3.5 flex items-center gap-2">
        <h2 className="text-[0.9375rem] leading-5 font-medium text-stone-900">{t('skills.problems')}</h2>
        <span className="grid h-[1.15rem] min-w-[1.15rem] place-items-center rounded-full bg-amber-100 px-1.5 text-[0.6875rem] font-medium text-amber-700">
          {diagnostics.length}
        </span>
      </div>
      <p className="mb-3 text-[0.8125rem] text-stone-500">{t('skills.problemsHint')}</p>
      <div className="overflow-hidden rounded-[18px] border border-amber-200/70 bg-amber-50/40">
        {diagnostics.map((diagnostic, index) => (
          <div key={`${diagnostic.path}-${index}`} className="border-b border-amber-200/50 px-4 py-3 last:border-b-0">
            <p className="truncate font-mono text-[0.71875rem] text-stone-500" title={diagnostic.path}>
              {relativizeHome(diagnostic.path)}
            </p>
            <p className="mt-0.5 text-[0.8125rem] leading-5 text-amber-800">{diagnostic.message}</p>
          </div>
        ))}
      </div>
    </section>
  )
}
