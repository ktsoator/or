import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  ArrowLeft,
  FileText,
  LoaderCircle,
  RefreshCw,
  X,
} from 'lucide-react'
import { useI18n } from '@/i18n'
import {
  fetchPromptTemplateDetail,
  fetchPromptTemplates,
  localizePromptTemplate,
  type PromptTemplateDetail,
  type PromptTemplateDiagnostic,
  type PromptTemplateEntry,
  type PromptTemplatesResponse,
} from './catalog'
import { cn } from '@/lib/utils'
import { Markdown } from '@/shared/ui/Markdown'
import { SidebarToggleButton } from '@/components/SidebarToggleButton'

function relativizeHome(path: string): string {
  const home = path.match(/^(\/(?:Users|home)\/[^/]+)/)?.[1]
  return home ? path.replace(home, '~') : path
}

export function PromptTemplatesPage({
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
  const { locale, t } = useI18n()
  const [data, setData] = useState<PromptTemplatesResponse>()
  const [error, setError] = useState(false)
  const [loading, setLoading] = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [selected, setSelected] = useState<PromptTemplateEntry>()

  const load = useCallback(async (refresh = false) => {
    if (refresh) setRefreshing(true)
    else setLoading(true)
    setError(false)
    try {
      setData(await fetchPromptTemplates(workspacePath))
    } catch {
      setError(true)
    } finally {
      setLoading(false)
      setRefreshing(false)
    }
  }, [workspacePath])

  useEffect(() => {
    void load()
  }, [load])

  const projectTemplates = useMemo(
    () => (data?.project ?? []).map((template) => localizePromptTemplate(template, locale)),
    [data?.project, locale],
  )
  const userTemplates = useMemo(
    () => (data?.user ?? []).map((template) => localizePromptTemplate(template, locale)),
    [data?.user, locale],
  )
  const total = projectTemplates.length + userTemplates.length

  return (
    <div className="flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-canvas">
      <header
        className={cn(
          'window-titlebar z-20 flex h-[45px] shrink-0 items-center gap-1 border-b border-edge/80 bg-canvas px-4 max-md:h-12',
          sidebarCollapsed && 'sidebar-is-collapsed',
        )}
      >
        {sidebarCollapsed && onExpandSidebar && (
          <SidebarToggleButton
            expanded={false}
            className="desktop-sidebar-toggle hidden md:grid"
            onToggle={onExpandSidebar}
          />
        )}
        <button
          className="window-titlebar-control flex h-9 cursor-pointer items-center gap-2 rounded-[8px] px-2.5 text-[0.84375rem] text-ink-muted outline-none transition-colors hover:bg-canvas-strong/65 hover:text-ink focus-visible:bg-canvas-strong/65 focus-visible:text-ink"
          type="button"
          onClick={onBack}
        >
          <ArrowLeft className="size-4" aria-hidden="true" />
          <span>{t('promptTemplates.back')}</span>
        </button>
      </header>

      <main className="min-h-0 flex-1 overflow-y-auto bg-canvas">
        <div className="mx-auto w-full max-w-[58rem] px-10 pt-12 pb-24 max-lg:px-7 max-md:px-4 max-md:pt-8">
          <div className="flex items-start justify-between gap-6">
            <div className="min-w-0">
              <div className="flex items-center gap-2.5">
                <h1 className="text-[1.75rem] leading-9 font-semibold text-ink max-md:text-[1.5rem]">
                  {t('promptTemplates.title')}
                </h1>
                {!loading && !error && total > 0 && (
                  <span className="mt-1 rounded-full bg-canvas-sunken px-2 py-0.5 text-[0.6875rem] font-medium text-ink-muted">
                    {total}
                  </span>
                )}
              </div>
              <p className="mt-1 max-w-[38rem] text-[0.84375rem] leading-5 text-ink-muted">
                {t('promptTemplates.subtitle')}
              </p>
            </div>
            <button
              className="mt-1 grid size-8 shrink-0 cursor-pointer place-items-center rounded-[8px] text-ink-muted outline-none transition-colors hover:bg-surface-hover hover:text-ink focus-visible:bg-surface-hover focus-visible:text-ink disabled:cursor-wait disabled:opacity-50"
              type="button"
              title={t('promptTemplates.refresh')}
              aria-label={t('promptTemplates.refresh')}
              disabled={loading || refreshing}
              onClick={() => void load(true)}
            >
              <RefreshCw
                className={cn('size-4', refreshing && 'animate-spin')}
                aria-hidden="true"
              />
            </button>
          </div>

          {loading ? (
            <div className="mt-16 flex items-center justify-center gap-2 text-[0.84375rem] text-ink-faint">
              <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              {t('promptTemplates.loading')}
            </div>
          ) : error ? (
            <div className="mt-12 flex flex-col items-center gap-3 rounded-[8px] border border-edge bg-canvas px-8 py-14 text-center">
              <p className="text-[0.84375rem] text-ink-muted">
                {t('promptTemplates.loadError')}
              </p>
              <button
                className="h-8 cursor-pointer rounded-[8px] border border-edge bg-canvas px-3.5 text-[0.8125rem] text-ink-soft transition-colors hover:bg-canvas-sunken"
                type="button"
                onClick={() => void load()}
              >
                {t('promptTemplates.retry')}
              </button>
            </div>
          ) : (
            <div className="mt-10 space-y-9">
              <PromptTemplateSection
                title={workspaceName
                  ? t('promptTemplates.projectSectionNamed', { name: workspaceName })
                  : t('promptTemplates.projectSection')}
                hint={workspacePath
                  ? relativizeHome(`${workspacePath}/.or/prompts`)
                  : t('promptTemplates.noProject')}
                templates={projectTemplates}
                empty={workspacePath
                  ? t('promptTemplates.emptyProject')
                  : t('promptTemplates.noProjectHint')}
                onSelect={setSelected}
              />
              <PromptTemplateSection
                title={t('promptTemplates.globalSection')}
                hint="~/.or/prompts"
                templates={userTemplates}
                empty={t('promptTemplates.emptyGlobal')}
                onSelect={setSelected}
              />
              {data && data.diagnostics.length > 0 && (
                <PromptTemplateDiagnostics diagnostics={data.diagnostics} />
              )}
            </div>
          )}
        </div>
      </main>

      {selected && (
        <PromptTemplateDetailDialog
          template={selected}
          workspacePath={workspacePath}
          onClose={() => setSelected(undefined)}
        />
      )}
    </div>
  )
}

function PromptTemplateSection({
  title,
  hint,
  templates,
  empty,
  onSelect,
}: {
  title: string
  hint: string
  templates: PromptTemplateEntry[]
  empty: string
  onSelect: (template: PromptTemplateEntry) => void
}) {
  return (
    <section>
      <div className="mb-3 flex items-center justify-between gap-4">
        <div className="flex shrink-0 items-center gap-2">
          <h2 className="text-[0.9375rem] leading-5 font-medium text-ink">{title}</h2>
          <span className="grid h-[1.15rem] min-w-[1.15rem] shrink-0 place-items-center rounded-full bg-canvas-sunken px-1.5 text-[0.6875rem] font-medium text-ink-muted">
            {templates.length}
          </span>
        </div>
        <span
          className="min-w-0 flex-1 truncate text-right font-mono text-[0.71875rem] text-ink-faint max-sm:hidden"
          title={hint}
        >
          {hint}
        </span>
      </div>
      {templates.length === 0 ? (
        <div className="rounded-[8px] border border-dashed border-edge bg-canvas-raised/40 px-6 py-9 text-center">
          <p className="mx-auto max-w-[24rem] text-[0.8125rem] leading-5 text-ink-faint">
            {empty}
          </p>
        </div>
      ) : (
        <div className="overflow-hidden rounded-[8px] border border-edge bg-canvas">
          {templates.map((template) => (
            <button
              key={`${template.source}-${template.name}`}
              type="button"
              className="grid min-h-14 w-full cursor-pointer grid-cols-[1.25rem_minmax(0,1fr)] items-center gap-x-3 gap-y-0.5 border-b border-edge/70 px-3.5 py-2.5 text-left outline-none transition-colors last:border-b-0 hover:bg-surface-hover focus-visible:bg-surface-hover sm:grid-cols-[1.25rem_minmax(8rem,0.65fr)_minmax(0,1.35fr)]"
              onClick={() => onSelect(template)}
            >
              <FileText className="size-4 shrink-0 text-ink-muted" strokeWidth={1.8} aria-hidden="true" />
              <span className="flex min-w-0 items-baseline gap-1.5">
                <span className="truncate font-mono text-[0.84375rem] font-medium text-ink">
                  {template.name}
                </span>
                {template.argumentHint && (
                  <span className="shrink-0 font-mono text-[0.6875rem] text-ink-faint">
                    {template.argumentHint}
                  </span>
                )}
              </span>
              <span className="col-start-2 min-w-0 truncate text-[0.8125rem] text-ink-muted sm:col-auto sm:text-right">
                {template.description}
              </span>
            </button>
          ))}
        </div>
      )}
    </section>
  )
}

function PromptTemplateDetailDialog({
  template,
  workspacePath,
  onClose,
}: {
  template: PromptTemplateEntry
  workspacePath?: string
  onClose: () => void
}) {
  const { locale, t } = useI18n()
  const [detail, setDetail] = useState<PromptTemplateDetail>()
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
    const controller = new AbortController()
    setLoading(true)
    setError(false)
    void fetchPromptTemplateDetail(template.name, workspacePath, controller.signal)
      .then(setDetail)
      .catch((loadError) => {
        if (loadError instanceof DOMException && loadError.name === 'AbortError') return
        setError(true)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [template.name, workspacePath])

  const localized = detail ? localizePromptTemplate(detail, locale) : template
  const sourceLabel = template.source === 'project'
    ? t('skills.systemSourceProject')
    : t('promptTemplates.globalSource')

  return (
    <div
      className="fixed inset-0 z-[140] grid place-items-center bg-scrim/20 px-4"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <section
        className="flex max-h-[min(85vh,48rem)] w-full max-w-[44rem] flex-col overflow-hidden rounded-[12px] border border-edge-strong/80 bg-canvas animate-[fade-in_100ms_ease-out]"
        role="dialog"
        aria-modal="true"
        aria-label={template.name}
      >
        <header className="flex items-start gap-3 border-b border-edge/70 px-5 py-4">
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2">
              <FileText className="size-4 shrink-0 text-ink-muted" strokeWidth={1.8} aria-hidden="true" />
              <h2 className="truncate font-mono text-[0.9375rem] font-semibold text-ink">
                {localized.name}
              </h2>
              {localized.argumentHint && (
                <span className="shrink-0 font-mono text-[0.6875rem] text-ink-faint">
                  {localized.argumentHint}
                </span>
              )}
              <span className="shrink-0 rounded-full bg-canvas-sunken px-2 py-0.5 text-[0.6875rem] font-medium text-ink-muted">
                {sourceLabel}
              </span>
            </div>
            <p className="mt-1 text-[0.8125rem] leading-5 text-ink-muted">
              {localized.description}
            </p>
            <p
              className="mt-1 truncate font-mono text-[0.6875rem] text-ink-faint"
              title={template.path}
            >
              {relativizeHome(template.path)}
            </p>
          </div>
          <button
            className="-mt-0.5 grid size-7 shrink-0 cursor-pointer place-items-center rounded-[8px] text-ink-faint transition-colors hover:bg-surface-hover hover:text-ink-soft"
            type="button"
            aria-label={t('promptTemplates.close')}
            onClick={onClose}
          >
            <X className="size-3.5" aria-hidden="true" />
          </button>
        </header>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {loading ? (
            <div className="flex items-center justify-center gap-2 py-10 text-[0.84375rem] text-ink-faint">
              <LoaderCircle className="size-4 animate-spin" aria-hidden="true" />
              {t('promptTemplates.loading')}
            </div>
          ) : error ? (
            <p className="py-10 text-center text-[0.84375rem] text-ink-muted">
              {t('promptTemplates.detailLoadError')}
            </p>
          ) : detail && detail.content.trim() ? (
            <div className="text-[0.875rem] leading-relaxed text-ink-soft">
              <Markdown source={detail.content} />
            </div>
          ) : (
            <p className="py-10 text-center text-[0.84375rem] text-ink-faint">
              {t('promptTemplates.emptyBody')}
            </p>
          )}
        </div>
      </section>
    </div>
  )
}

function PromptTemplateDiagnostics({
  diagnostics,
}: {
  diagnostics: PromptTemplateDiagnostic[]
}) {
  const { t } = useI18n()
  return (
    <section>
      <div className="mb-3 flex items-center gap-2">
        <h2 className="text-[0.9375rem] leading-5 font-medium text-ink">
          {t('promptTemplates.problems')}
        </h2>
        <span className="grid h-[1.15rem] min-w-[1.15rem] place-items-center rounded-full bg-warning-surface px-1.5 text-[0.6875rem] font-medium text-warning">
          {diagnostics.length}
        </span>
      </div>
      <p className="mb-3 text-[0.8125rem] text-ink-muted">
        {t('promptTemplates.problemsHint')}
      </p>
      <div className="overflow-hidden rounded-[8px] border border-warning-edge/70 bg-warning-surface/40">
        {diagnostics.map((diagnostic, index) => (
          <div
            key={`${diagnostic.path}-${index}`}
            className="border-b border-warning-edge/50 px-4 py-3 last:border-b-0"
          >
            <p
              className="truncate font-mono text-[0.71875rem] text-ink-muted"
              title={diagnostic.path}
            >
              {relativizeHome(diagnostic.path)}
            </p>
            <p className="mt-0.5 text-[0.8125rem] leading-5 text-warning">
              {diagnostic.message}
            </p>
          </div>
        ))}
      </div>
    </section>
  )
}
