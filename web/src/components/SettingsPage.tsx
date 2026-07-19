import { useMemo, useState, type ReactNode } from 'react'
import type { LucideIcon } from 'lucide-react'
import {
  Archive,
  ArrowLeft,
  Cable,
  Check,
  ChevronDown,
  Cpu,
  Gauge,
  Keyboard,
  Search,
  Settings2,
  ShieldCheck,
  Sun,
  UserRound,
  Wrench,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import { cn } from '@/lib/utils'
import { useI18n, type Locale } from '@/i18n'
import { UsageSettings } from '@/components/UsageSettings'
import {
  readAppearancePreferences,
  saveAppearancePreferences,
  type AppearancePreferences,
  type InterfaceDensity,
  type TextSize,
} from '@/lib/appearance'

export type SettingsSection =
  | 'general'
  | 'usage'
  | 'profile'
  | 'appearance'
  | 'models'
  | 'permissions'
  | 'keyboard'
  | 'tools'
  | 'connections'
  | 'archived'

type NavItem = {
  id: SettingsSection
  label: string
  icon: LucideIcon
}

type NavGroup = {
  label: string
  items: NavItem[]
}

export function SettingsPage({
  onBack,
  initialSection = 'general',
}: {
  onBack: () => void
  initialSection?: SettingsSection
}) {
  const { t } = useI18n()
  const [active, setActive] = useState<SettingsSection>(initialSection)
  const [query, setQuery] = useState('')
  const [fileChanges, setFileChanges] = useState(true)
  const [commands, setCommands] = useState(true)
  const [readAccess, setReadAccess] = useState(true)
  const [responseUsage, setResponseUsage] = useState(true)
  const [backgroundRuns, setBackgroundRuns] = useState(true)
  const [fileOpener, setFileOpener] = useState('vscode')
  const [toolResults, setToolResults] = useState('collapsed')
  const [appearance, setAppearance] = useState(readAppearancePreferences)

  const groups = useMemo<NavGroup[]>(
    () => [
      {
        label: t('settings.personal'),
        items: [
          { id: 'general', label: t('settings.general'), icon: Settings2 },
          { id: 'profile', label: t('settings.profile'), icon: UserRound },
          { id: 'usage', label: t('settings.usage'), icon: Gauge },
          { id: 'appearance', label: t('settings.appearance'), icon: Sun },
          { id: 'models', label: t('settings.models'), icon: Cpu },
          { id: 'permissions', label: t('settings.permissions'), icon: ShieldCheck },
          { id: 'keyboard', label: t('settings.keyboard'), icon: Keyboard },
        ],
      },
      {
        label: t('settings.workspaceSection'),
        items: [
          { id: 'tools', label: t('settings.tools'), icon: Wrench },
          { id: 'connections', label: t('settings.connections'), icon: Cable },
        ],
      },
      {
        label: t('settings.archived'),
        items: [
          { id: 'archived', label: t('settings.archivedSessions'), icon: Archive },
        ],
      },
    ],
    [t],
  )
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const visibleGroups = normalizedQuery
    ? groups
        .map((group) => ({
          ...group,
          items: group.items.filter((item) =>
            item.label.toLocaleLowerCase().includes(normalizedQuery),
          ),
        }))
        .filter((group) => group.items.length > 0)
    : groups
  const activeItem = groups.flatMap((group) => group.items).find((item) => item.id === active)
  const permissionState = {
    fileChanges,
    commands,
    readAccess,
    setFileChanges,
    setCommands,
    setReadAccess,
  }
  const updateAppearance = (next: AppearancePreferences) => {
    setAppearance(next)
    saveAppearancePreferences(next)
  }

  return (
    <div className="grid h-full min-h-0 grid-cols-[16rem_minmax(0,1fr)] overflow-hidden bg-white max-md:grid-cols-1 max-md:grid-rows-[auto_minmax(0,1fr)]">
      <aside className="flex min-h-0 flex-col border-r border-stone-200/80 bg-[#fbfbfa] px-3 py-4 max-md:border-r-0 max-md:border-b max-md:px-3 max-md:py-2.5">
        <button
          className="flex h-9 w-full cursor-pointer items-center gap-2 rounded-[10px] px-2.5 text-[0.84375rem] font-normal text-stone-500 outline-none transition-colors hover:bg-stone-200/65 hover:text-stone-900 focus-visible:ring-2 focus-visible:ring-stone-300 max-md:w-fit"
          type="button"
          onClick={onBack}
        >
          <ArrowLeft className="size-4" aria-hidden="true" />
          <span>{t('settings.back')}</span>
        </button>

        <label className="relative mt-3 block max-md:absolute max-md:top-2.5 max-md:right-3 max-md:mt-0 max-md:w-[min(48vw,15rem)]">
          <Search
            className="pointer-events-none absolute top-1/2 left-2.5 size-3.5 -translate-y-1/2 text-stone-400"
            aria-hidden="true"
          />
          <input
            className="h-8 w-full rounded-[10px] border border-stone-200 bg-white pr-2.5 pl-8 text-[0.8125rem] text-stone-800 outline-none transition-[border-color,box-shadow] placeholder:text-stone-400 focus:border-stone-400 focus:ring-2 focus:ring-stone-200"
            type="search"
            value={query}
            placeholder={t('settings.search')}
            aria-label={t('settings.search')}
            onChange={(event) => setQuery(event.target.value)}
          />
        </label>

        <nav className="mt-4 min-h-0 flex-1 overflow-y-auto pb-3 max-md:mt-3 max-md:flex max-md:max-w-full max-md:gap-1 max-md:overflow-x-auto max-md:overflow-y-hidden max-md:pb-0">
          {visibleGroups.map((group) => (
            <div key={group.label} className="mt-4 first:mt-0 max-md:mt-0 max-md:flex max-md:gap-1">
              <div className="mb-1 px-2 text-[0.71875rem] font-medium text-stone-400 max-md:hidden">
                {group.label}
              </div>
              {group.items.map((item) => (
                <SettingsNavItem
                  key={item.id}
                  item={item}
                  active={item.id === active}
                  onClick={() => {
                    setActive(item.id)
                    setQuery('')
                  }}
                />
              ))}
            </div>
          ))}
          {visibleGroups.length === 0 && (
            <div className="px-2 py-4 text-[0.78125rem] text-stone-400">
              {t('settings.noResults')}
            </div>
          )}
        </nav>
      </aside>

      <main className="min-h-0 overflow-y-auto bg-white">
        <div className="mx-auto w-full max-w-[58.75rem] px-10 pt-14 pb-24 max-lg:px-7 max-md:px-4 max-md:pt-7">
          <h1 className="text-[1.75rem] leading-9 font-semibold tracking-[-0.035em] text-stone-950 max-md:text-[1.5rem]">
            {activeItem?.label ?? t('settings.general')}
          </h1>

          <div className={active === 'usage' ? 'mt-8 max-md:mt-6' : 'mt-11 max-md:mt-7'}>
            {active === 'general' ? (
              <GeneralSettings
                permissionState={permissionState}
                fileOpener={fileOpener}
                toolResults={toolResults}
                responseUsage={responseUsage}
                backgroundRuns={backgroundRuns}
                onFileOpenerChange={setFileOpener}
                onToolResultsChange={setToolResults}
                onResponseUsageChange={setResponseUsage}
                onBackgroundRunsChange={setBackgroundRuns}
              />
            ) : active === 'permissions' ? (
              <PermissionsCard {...permissionState} />
            ) : active === 'usage' ? (
              <UsageSettings />
            ) : active === 'appearance' ? (
              <AppearanceSettings preferences={appearance} onChange={updateAppearance} />
            ) : (
              <SettingsPlaceholder section={activeItem?.label ?? ''} icon={activeItem?.icon} />
            )}
          </div>
        </div>
      </main>
    </div>
  )
}

function SettingsNavItem({
  item,
  active,
  onClick,
}: {
  item: NavItem
  active: boolean
  onClick: () => void
}) {
  const Icon = item.icon
  return (
    <button
      className={cn(
        'mb-0.5 flex h-9 w-full cursor-pointer items-center gap-2.5 rounded-[10px] px-2.5 text-left text-[0.84375rem] font-normal text-stone-700 outline-none transition-colors hover:bg-stone-200/60 hover:text-stone-950 focus-visible:ring-2 focus-visible:ring-stone-300 max-md:mb-0 max-md:w-auto max-md:shrink-0 max-md:pr-3',
        active && 'bg-[rgb(237,237,237)] text-stone-950 hover:bg-[rgb(237,237,237)]',
      )}
      type="button"
      aria-current={active ? 'page' : undefined}
      onClick={onClick}
    >
      <Icon className="size-4 shrink-0" strokeWidth={1.8} aria-hidden="true" />
      <span className="whitespace-nowrap">{item.label}</span>
    </button>
  )
}

function AppearanceSettings({
  preferences,
  onChange,
}: {
  preferences: AppearancePreferences
  onChange: (preferences: AppearancePreferences) => void
}) {
  const { t } = useI18n()
  const densityOptions: Array<{ value: InterfaceDensity; label: string }> = [
    { value: 'compact', label: t('settings.compact') },
    { value: 'default', label: t('settings.defaultSize') },
    { value: 'comfortable', label: t('settings.comfortable') },
  ]
  const textOptions: Array<{ value: TextSize; label: string }> = [
    { value: 'small', label: t('settings.small') },
    { value: 'default', label: t('settings.defaultSize') },
    { value: 'large', label: t('settings.large') },
  ]

  return (
    <SettingsSection title={t('settings.display')}>
      <SettingsCard>
        <SettingsRow
          label={t('settings.interfaceDensity')}
          description={t('settings.interfaceDensityDescription')}
          control={
            <SelectControl
              value={preferences.density}
              ariaLabel={t('settings.interfaceDensity')}
              options={densityOptions}
              onChange={(density) =>
                onChange({ ...preferences, density: density as InterfaceDensity })
              }
            />
          }
        />
        <SettingsRow
          label={t('settings.chatText')}
          description={t('settings.chatTextDescription')}
          control={
            <SelectControl
              value={preferences.chatText}
              ariaLabel={t('settings.chatText')}
              options={textOptions}
              onChange={(chatText) =>
                onChange({ ...preferences, chatText: chatText as TextSize })
              }
            />
          }
        />
        <SettingsRow
          label={t('settings.codeText')}
          description={t('settings.codeTextDescription')}
          control={
            <SelectControl
              value={preferences.codeText}
              ariaLabel={t('settings.codeText')}
              options={textOptions}
              onChange={(codeText) =>
                onChange({ ...preferences, codeText: codeText as TextSize })
              }
            />
          }
        />
      </SettingsCard>
    </SettingsSection>
  )
}

function GeneralSettings({
  permissionState,
  fileOpener,
  toolResults,
  responseUsage,
  backgroundRuns,
  onFileOpenerChange,
  onToolResultsChange,
  onResponseUsageChange,
  onBackgroundRunsChange,
}: {
  permissionState: PermissionState
  fileOpener: string
  toolResults: string
  responseUsage: boolean
  backgroundRuns: boolean
  onFileOpenerChange: (value: string) => void
  onToolResultsChange: (value: string) => void
  onResponseUsageChange: (value: boolean) => void
  onBackgroundRunsChange: (value: boolean) => void
}) {
  const { locale, setLocale, t } = useI18n()
  return (
    <div className="space-y-11">
      <SettingsSection title={t('settings.permissionsTitle')}>
        <PermissionsCard {...permissionState} embedded />
      </SettingsSection>

      <SettingsSection title={t('settings.generalTitle')}>
        <SettingsCard>
          <SettingsRow
            label={t('settings.language')}
            description={t('settings.languageDescription')}
            control={
              <SelectControl
                value={locale}
                ariaLabel={t('settings.language')}
                searchPlaceholder={t('settings.searchLanguages')}
                onChange={(value) => setLocale(value as Locale)}
                options={[
                  { value: 'en', label: t('profile.english') },
                  { value: 'zh-CN', label: t('profile.chinese') },
                ]}
              />
            }
          />
          <SettingsRow
            label={t('settings.fileOpener')}
            description={t('settings.fileOpenerDescription')}
            control={
              <SelectControl
                value={fileOpener}
                ariaLabel={t('settings.fileOpener')}
                onChange={onFileOpenerChange}
                options={[
                  { value: 'vscode', label: 'VS Code' },
                  { value: 'system', label: t('settings.systemDefault') },
                ]}
              />
            }
          />
          <SettingsRow
            label={t('settings.toolResults')}
            description={t('settings.toolResultsDescription')}
            control={
              <SelectControl
                value={toolResults}
                ariaLabel={t('settings.toolResults')}
                onChange={onToolResultsChange}
                options={[
                  { value: 'collapsed', label: t('settings.collapsed') },
                  { value: 'expanded', label: t('settings.expanded') },
                ]}
              />
            }
          />
          <SettingsRow
            label={t('settings.responseUsage')}
            description={t('settings.responseUsageDescription')}
            control={
              <Toggle
                checked={responseUsage}
                label={t('settings.responseUsage')}
                onCheckedChange={onResponseUsageChange}
              />
            }
          />
          <SettingsRow
            label={t('settings.backgroundRuns')}
            description={t('settings.backgroundRunsDescription')}
            control={
              <Toggle
                checked={backgroundRuns}
                label={t('settings.backgroundRuns')}
                onCheckedChange={onBackgroundRunsChange}
              />
            }
          />
        </SettingsCard>
      </SettingsSection>
    </div>
  )
}

type PermissionState = {
  fileChanges: boolean
  commands: boolean
  readAccess: boolean
  setFileChanges: (value: boolean) => void
  setCommands: (value: boolean) => void
  setReadAccess: (value: boolean) => void
}

function PermissionsCard({
  fileChanges,
  commands,
  readAccess,
  setFileChanges,
  setCommands,
  setReadAccess,
  embedded = false,
}: PermissionState & { embedded?: boolean }) {
  const { t } = useI18n()
  const card = (
    <SettingsCard>
      <SettingsRow
        label={t('settings.fileChanges')}
        description={t('settings.fileChangesDescription')}
        control={
          <Toggle
            checked={fileChanges}
            label={t('settings.fileChanges')}
            onCheckedChange={setFileChanges}
          />
        }
      />
      <SettingsRow
        label={t('settings.commands')}
        description={t('settings.commandsDescription')}
        control={
          <Toggle
            checked={commands}
            label={t('settings.commands')}
            onCheckedChange={setCommands}
          />
        }
      />
      <SettingsRow
        label={t('settings.readAccess')}
        description={t('settings.readAccessDescription')}
        control={
          <Toggle
            checked={readAccess}
            label={t('settings.readAccess')}
            onCheckedChange={setReadAccess}
          />
        }
      />
    </SettingsCard>
  )
  if (embedded) return card
  return <SettingsSection title={t('settings.permissionsTitle')}>{card}</SettingsSection>
}

function SettingsSection({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section>
      <h2 className="mb-3 text-[0.875rem] leading-5 font-medium text-stone-800">{title}</h2>
      {children}
    </section>
  )
}

function SettingsCard({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-hidden rounded-[18px] border border-stone-200/90 bg-white px-4 shadow-[0_10px_32px_-30px_rgba(28,25,23,0.45)] max-md:px-3.5">
      {children}
    </div>
  )
}

function SettingsRow({
  label,
  description,
  control,
}: {
  label: string
  description: string
  control: ReactNode
}) {
  return (
    <div className="flex min-h-[4.375rem] items-center gap-6 border-b border-stone-200/75 py-3 last:border-b-0 max-sm:items-start max-sm:gap-3">
      <div className="min-w-0 flex-1">
        <div className="text-[0.84375rem] leading-5 font-medium text-stone-900">{label}</div>
        <p className="mt-0.5 max-w-[38.75rem] text-[0.78125rem] leading-[1.45] text-stone-500">
          {description}
        </p>
      </div>
      <div className="shrink-0 max-sm:pt-0.5">{control}</div>
    </div>
  )
}

function Toggle({
  checked,
  label,
  onCheckedChange,
}: {
  checked: boolean
  label: string
  onCheckedChange: (checked: boolean) => void
}) {
  return (
    <button
      className={cn(
        'relative h-[1.375rem] w-9 cursor-pointer rounded-full outline-none transition-colors focus-visible:ring-2 focus-visible:ring-stone-400 focus-visible:ring-offset-2',
        checked ? 'bg-stone-900' : 'bg-stone-300',
      )}
      type="button"
      role="switch"
      aria-label={label}
      aria-checked={checked}
      onClick={() => onCheckedChange(!checked)}
    >
      <span
        className={cn(
          'absolute top-[0.1875rem] left-[0.1875rem] size-4 rounded-full bg-white shadow-sm transition-transform duration-150 ease-out',
          checked && 'translate-x-[0.875rem]',
        )}
        aria-hidden="true"
      />
    </button>
  )
}

function SelectControl({
  value,
  options,
  ariaLabel,
  searchPlaceholder,
  onChange,
}: {
  value: string
  options: Array<{ value: string; label: string }>
  ariaLabel: string
  searchPlaceholder?: string
  onChange: (value: string) => void
}) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const selected = options.find((option) => option.value === value) ?? options[0]
  const normalizedQuery = query.trim().toLocaleLowerCase()
  const visibleOptions = normalizedQuery
    ? options.filter((option) => option.label.toLocaleLowerCase().includes(normalizedQuery))
    : options

  return (
    <DropdownMenu.Root
      open={open}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) setQuery('')
      }}
    >
      <DropdownMenu.Trigger asChild>
        <button
          className="group inline-flex h-9 min-w-[9.75rem] max-w-[14rem] cursor-pointer items-center justify-between gap-2 rounded-[11px] border border-stone-200 bg-white px-3 text-[0.8125rem] font-normal text-stone-800 outline-none transition-[background-color,border-color,box-shadow] hover:bg-[rgb(241,241,241)] focus-visible:border-stone-400 focus-visible:ring-2 focus-visible:ring-stone-200 data-[state=open]:bg-[rgb(237,237,237)] max-sm:min-w-[7.75rem] max-sm:max-w-[9.75rem]"
          type="button"
          aria-label={ariaLabel}
        >
          <span className="min-w-0 truncate">{selected?.label ?? '—'}</span>
          <ChevronDown
            className="size-3.5 shrink-0 text-stone-400 transition-transform duration-150 group-data-[state=open]:rotate-180"
            aria-hidden="true"
          />
        </button>
      </DropdownMenu.Trigger>

      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="bottom"
          align="end"
          sideOffset={6}
          collisionPadding={10}
          className={cn(
            'z-[120] max-h-[min(420px,var(--radix-dropdown-menu-content-available-height))] min-w-[var(--radix-dropdown-menu-trigger-width)] animate-[fade-in_110ms_ease-out] overflow-y-auto rounded-2xl border border-stone-200 bg-white p-1 text-[0.84375rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none',
            searchPlaceholder && 'w-[17.5rem] max-w-[calc(100vw-1.25rem)]',
          )}
        >
          {searchPlaceholder && (
            <div className="relative mb-1 border-b border-stone-100 px-1 pb-1">
              <Search
                className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-[calc(50%+2px)] text-stone-400"
                strokeWidth={1.8}
                aria-hidden="true"
              />
              <input
                autoFocus
                className="h-9 w-full rounded-[10px] bg-transparent pr-2 pl-8 text-[0.84375rem] text-stone-900 outline-none placeholder:text-stone-400"
                type="search"
                value={query}
                placeholder={searchPlaceholder}
                aria-label={searchPlaceholder}
                onChange={(event) => setQuery(event.target.value)}
                onKeyDown={(event) => event.stopPropagation()}
              />
            </div>
          )}

          <DropdownMenu.RadioGroup
            className="flex flex-col gap-0.5"
            value={value}
            onValueChange={onChange}
          >
            {visibleOptions.map((option) => (
              <DropdownMenu.RadioItem
                key={option.value}
                value={option.value}
                className="relative flex h-9 cursor-default select-none items-center rounded-[10px] px-2.5 pr-9 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)] data-[state=checked]:font-medium"
              >
                <span className="min-w-0 flex-1 truncate">{option.label}</span>
                <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-stone-700">
                  <Check className="size-3.5" aria-hidden="true" />
                </DropdownMenu.ItemIndicator>
              </DropdownMenu.RadioItem>
            ))}
          </DropdownMenu.RadioGroup>

          {visibleOptions.length === 0 && (
            <div className="px-2.5 py-3 text-center text-[0.78125rem] text-stone-400">
              {t('settings.noResults')}
            </div>
          )}
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}

function SettingsPlaceholder({ section, icon: Icon }: { section: string; icon?: LucideIcon }) {
  const { t } = useI18n()
  return (
    <div className="flex min-h-[16.25rem] flex-col items-center justify-center rounded-[20px] border border-stone-200/90 bg-[#fdfdfc] px-8 text-center">
      {Icon && (
        <div className="grid size-10 place-items-center rounded-xl bg-stone-100 text-stone-500">
          <Icon className="size-5" strokeWidth={1.7} aria-hidden="true" />
        </div>
      )}
      <h2 className="mt-4 text-[1rem] font-medium text-stone-900">
        {t('settings.previewTitle', { section })}
      </h2>
      <p className="mt-1.5 text-[0.8125rem] leading-5 text-stone-500">
        {t('settings.previewDescription')}
      </p>
    </div>
  )
}
