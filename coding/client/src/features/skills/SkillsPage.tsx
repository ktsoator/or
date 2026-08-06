import { useCallback, useEffect, useState } from 'react'
import { ArrowLeft, LoaderCircle, X } from 'lucide-react'
import { apiURL } from '@/api'
import { useI18n } from '@/i18n'
import {
  fetchSkills,
  type SkillDiagnostic,
  type SkillEntry,
  type SkillsResponse,
} from './catalog'
import { Markdown } from '@/shared/ui/Markdown'
import { SidebarToggleButton } from '@/components/SidebarToggleButton'

// relativizeHome trims the user's home prefix off an absolute path so directory
// hints read as "~/.agents/skills/commit" rather than a long absolute path.
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
      setData(await fetchSkills(workspacePath))
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
    <div className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-canvas">
      <header
        className={`skills-header window-titlebar z-20 flex h-[45px] shrink-0 items-center gap-1 border-b border-edge/80 bg-canvas px-4 max-md:h-12 ${sidebarCollapsed ? 'sidebar-is-collapsed' : ''}`}
      >
        {sidebarCollapsed && onExpandSidebar && (
          <SidebarToggleButton
            expanded={false}
            className="desktop-sidebar-toggle hidden md:grid"
            onToggle={onExpandSidebar}
          />
        )}
        <button
          className="window-titlebar-control flex h-9 cursor-pointer items-center gap-2 rounded-[10px] px-2.5 text-[0.84375rem] font-normal text-ink-muted outline-none transition-colors hover:bg-canvas-strong/65 hover:text-ink focus-visible:bg-canvas-strong/65 focus-visible:text-ink"
          type="button"
          onClick={onBack}
        >
          <ArrowLeft className="size-4" aria-hidden="true" />
          <span>{t('skills.back')}</span>
        </button>
      </header>

      <main className="min-h-0 flex-1 overflow-y-auto bg-canvas">
        <div className="mx-auto w-full max-w-[56rem] px-10 pt-12 pb-24 max-lg:px-7 max-md:px-4 max-md:pt-8">
          <div className="flex items-center gap-2.5">
            <h1 className="text-[1.75rem] leading-9 font-semibold tracking-[-0.035em] text-ink max-md:text-[1.5rem]">
              {t('skills.title')}
            </h1>
            {!loading && !error && total > 0 && (
              <span className="mt-1 rounded-full bg-canvas-sunken px-2 py-0.5 text-[0.6875rem] font-medium text-ink-muted">
                {total}
              </span>
            )}
          </div>
          <p className="mt-1 max-w-[34rem] text-[0.84375rem] leading-5 text-ink-muted">
            {t('skills.subtitle')}
          </p>

          {loading ? (
            <div className="mt-16 flex items-center justify-center gap-2 text-[0.84375rem] text-ink-faint">
              <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              {t('skills.loading')}
            </div>
          ) : error ? (
            <div className="mt-12 flex flex-col items-center gap-3 rounded-[20px] border border-edge/90 bg-canvas px-8 py-14 text-center shadow-[0_10px_32px_-30px_rgba(28,25,23,0.45)]">
              <p className="text-[0.84375rem] text-ink-muted">{t('skills.loadError')}</p>
              <button
                className="h-8 cursor-pointer rounded-[10px] border border-edge bg-canvas px-3.5 text-[0.8125rem] text-ink-soft transition-colors hover:bg-canvas-sunken"
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
                hint={workspacePath ? relativizeHome(`${workspacePath}/.agents/skills`) : t('skills.noProject')}
                skills={data?.project ?? []}
                empty={workspacePath ? t('skills.emptyProject') : t('skills.noProjectHint')}
                onSelect={setSelected}
              />
              <SkillSection
                title={t('skills.systemSection')}
                hint="~/.agents/skills"
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
          <h2 className="text-[0.9375rem] leading-5 font-medium text-ink">{title}</h2>
          <span className="grid h-[1.15rem] min-w-[1.15rem] shrink-0 place-items-center rounded-full bg-canvas-sunken px-1.5 text-[0.6875rem] font-medium text-ink-muted">
            {skills.length}
          </span>
        </div>
        <span className="min-w-0 flex-1 truncate text-right font-mono text-[0.71875rem] text-ink-faint max-sm:hidden" title={hint}>
          {hint}
        </span>
      </div>
      {skills.length === 0 ? (
        <div className="rounded-[20px] border border-dashed border-edge bg-canvas-raised/50 px-6 py-11 text-center">
          <p className="mx-auto max-w-[22rem] text-[0.8125rem] leading-5 text-ink-faint">{empty}</p>
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
      className="flex cursor-pointer flex-col gap-0.5 rounded-[18px] border border-edge bg-canvas px-4 py-3.5 text-left outline-none transition-colors hover:border-edge-strong hover:bg-canvas-raised/60 focus-visible:border-edge-stronger focus-visible:bg-canvas-raised/60"
      title={relativizeHome(skill.dir)}
      onClick={() => onSelect(skill)}
    >
      <div className="flex min-w-0 items-center gap-2">
        <div className="min-w-0 flex-1 truncate font-mono text-[0.84375rem] font-medium text-ink">
          {skill.name}
        </div>
      </div>
      <p className="line-clamp-2 text-[0.8125rem] leading-[1.45] text-ink-muted">{skill.description}</p>
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
      className="fixed inset-0 z-[140] grid place-items-center bg-scrim/20 px-4"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <section
        className="flex max-h-[min(85vh,48rem)] w-full max-w-[44rem] flex-col overflow-hidden rounded-[16px] border border-edge-strong/80 bg-canvas shadow-[0_24px_64px_-28px_rgba(28,25,23,0.42)] animate-[fade-in_100ms_ease-out]"
        role="dialog"
        aria-modal="true"
        aria-label={skill.name}
      >
        <header className="flex items-start gap-3 border-b border-edge/70 px-5 py-4">
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <h2 className="truncate font-mono text-[0.9375rem] font-semibold text-ink">{skill.name}</h2>
              <span className="shrink-0 rounded-full bg-canvas-sunken px-2 py-0.5 text-[0.6875rem] font-medium text-ink-muted">
                {sourceLabel}
              </span>
            </div>
            <p className="mt-1 text-[0.8125rem] leading-5 text-ink-muted">{skill.description}</p>
            <p className="mt-1 truncate font-mono text-[0.6875rem] text-ink-faint" title={skill.dir}>
              {relativizeHome(skill.dir)}
            </p>
          </div>
          <button
            className="-mt-0.5 grid size-7 shrink-0 cursor-pointer place-items-center rounded-[8px] text-ink-faint transition-colors hover:bg-surface-hover hover:text-ink-soft"
            type="button"
            aria-label={t('skills.close')}
            onClick={onClose}
          >
            <X className="size-3.5" aria-hidden="true" />
          </button>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {loading ? (
            <div className="flex items-center justify-center gap-2 py-10 text-[0.84375rem] text-ink-faint">
              <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              {t('skills.loading')}
            </div>
          ) : error ? (
            <p className="py-10 text-center text-[0.84375rem] text-ink-muted">{t('skills.loadError')}</p>
          ) : detail && detail.content.trim() ? (
            <div className="text-[0.875rem] leading-relaxed text-ink-soft">
              <Markdown source={detail.content} />
            </div>
          ) : (
            <p className="py-10 text-center text-[0.84375rem] text-ink-faint">{t('skills.emptyBody')}</p>
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
        <h2 className="text-[0.9375rem] leading-5 font-medium text-ink">{t('skills.problems')}</h2>
        <span className="grid h-[1.15rem] min-w-[1.15rem] place-items-center rounded-full bg-warning-surface px-1.5 text-[0.6875rem] font-medium text-warning">
          {diagnostics.length}
        </span>
      </div>
      <p className="mb-3 text-[0.8125rem] text-ink-muted">{t('skills.problemsHint')}</p>
      <div className="overflow-hidden rounded-[18px] border border-warning-edge/70 bg-warning-surface/40">
        {diagnostics.map((diagnostic, index) => (
          <div key={`${diagnostic.path}-${index}`} className="border-b border-warning-edge/50 px-4 py-3 last:border-b-0">
            <p className="truncate font-mono text-[0.71875rem] text-ink-muted" title={diagnostic.path}>
              {relativizeHome(diagnostic.path)}
            </p>
            <p className="mt-0.5 text-[0.8125rem] leading-5 text-warning">{diagnostic.message}</p>
          </div>
        ))}
      </div>
    </section>
  )
}
