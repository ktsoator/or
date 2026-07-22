import { useMemo, useState } from 'react'
import { Check, ChevronDown, ChevronRight, Folder, FolderOpen, Plus, Search } from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import type { WorkspaceSummary } from '@/types'
import { useI18n } from '@/i18n'

export function ProjectPicker({
  workspaces,
  selectedPath,
  disabled,
  onSelect,
  onBrowse,
}: {
  workspaces: WorkspaceSummary[]
  selectedPath?: string
  disabled: boolean
  onSelect: (path?: string) => void
  onBrowse: () => void
}) {
  const { t } = useI18n()
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const selected = workspaces.find((workspace) => workspace.path === selectedPath)
  const filtered = useMemo(() => {
    const normalized = query.trim().toLocaleLowerCase()
    if (!normalized) return workspaces
    return workspaces.filter(
      (workspace) =>
        workspace.name.toLocaleLowerCase().includes(normalized) ||
        workspace.path.toLocaleLowerCase().includes(normalized),
    )
  }, [query, workspaces])

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
          type="button"
          className="group inline-flex h-8 max-w-full cursor-pointer items-center gap-2 rounded-xl px-2.5 text-[0.875rem] font-medium text-stone-800 outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(237,237,237)] disabled:cursor-not-allowed disabled:opacity-45 max-sm:size-9 max-sm:justify-center max-sm:p-0"
          aria-label={t('workspace.chooseProject')}
          disabled={disabled}
        >
          <Folder className="size-[1.0625rem] shrink-0" strokeWidth={1.8} aria-hidden="true" />
          <span className="min-w-0 truncate max-sm:hidden">
            {selected?.name ?? t('workspace.chooseProject')}
          </span>
          <ChevronDown
            className="size-3.5 shrink-0 text-stone-400 transition-transform duration-150 group-data-[state=open]:rotate-180 max-sm:hidden"
            aria-hidden="true"
          />
        </button>
      </DropdownMenu.Trigger>

      <DropdownMenu.Portal>
        <DropdownMenu.Content
          side="top"
          align="start"
          sideOffset={8}
          collisionPadding={12}
          className="z-[110] w-[19rem] max-w-[calc(100vw-24px)] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1.5 text-[0.875rem] text-stone-900 shadow-[0_18px_48px_-26px_rgba(28,25,23,0.5)] outline-none"
        >
          <div
            className="relative mb-1"
            onKeyDown={(event) => event.stopPropagation()}
          >
            <Search
              className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-stone-400"
              aria-hidden="true"
            />
            <input
              className="h-9 w-full rounded-[10px] border-0 bg-transparent pr-2.5 pl-8.5 text-[0.875rem] outline-none placeholder:text-stone-400 focus:bg-stone-50"
              value={query}
              placeholder={t('workspace.searchProjects')}
              aria-label={t('workspace.searchProjects')}
              autoFocus
              onChange={(event) => setQuery(event.target.value)}
            />
          </div>

          <DropdownMenu.Separator className="mx-1 h-px bg-stone-200/80" />
          <DropdownMenu.RadioGroup
            value={selectedPath ?? ''}
            onValueChange={(path) => onSelect(path)}
            className="my-1 flex max-h-[14rem] flex-col gap-0.5 overflow-y-auto"
          >
            {filtered.length > 0 ? (
              filtered.map((workspace) => (
                <DropdownMenu.RadioItem
                  key={workspace.path}
                  value={workspace.path}
                  className="relative flex h-9 cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 pr-8 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)]"
                  title={workspace.path}
                >
                  <Folder className="size-[1.0625rem] shrink-0 text-stone-600" strokeWidth={1.7} aria-hidden="true" />
                  <span className="min-w-0 flex-1 truncate">{workspace.name}</span>
                  <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-stone-700">
                    <Check className="size-3.5" aria-hidden="true" />
                  </DropdownMenu.ItemIndicator>
                </DropdownMenu.RadioItem>
              ))
            ) : (
              <div className="flex h-10 items-center px-2.5 text-[0.8125rem] text-stone-400">
                {t('workspace.noMatchingProjects')}
              </div>
            )}
          </DropdownMenu.RadioGroup>

          <DropdownMenu.Separator className="mx-1 h-px bg-stone-200/80" />
          <DropdownMenu.Sub>
            <DropdownMenu.SubTrigger className="mt-1 flex h-9 cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=open]:bg-[rgb(237,237,237)]">
              <Plus className="size-[1.0625rem] shrink-0" aria-hidden="true" />
              <span>{t('workspace.newProject')}</span>
              <ChevronRight className="ml-auto size-3.5 text-stone-400" aria-hidden="true" />
            </DropdownMenu.SubTrigger>
            <DropdownMenu.Portal>
              <DropdownMenu.SubContent
                sideOffset={6}
                alignOffset={-5}
                collisionPadding={12}
                className="z-[120] flex min-w-[13.75rem] animate-[fade-in_110ms_ease-out] flex-col gap-0.5 rounded-2xl border border-stone-200 bg-white p-1.5 text-[0.875rem] text-stone-900 shadow-[0_18px_48px_-26px_rgba(28,25,23,0.5)] outline-none"
              >
                <DropdownMenu.Item
                  className="flex h-9 cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]"
                  onSelect={() => onSelect(undefined)}
                >
                  <Plus className="size-[1.0625rem]" aria-hidden="true" />
                  {t('workspace.startFromScratch')}
                </DropdownMenu.Item>
                <DropdownMenu.Item
                  className="flex h-9 cursor-default select-none items-center gap-2 rounded-[10px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]"
                  onSelect={onBrowse}
                >
                  <FolderOpen className="size-[1.0625rem]" strokeWidth={1.7} aria-hidden="true" />
                  {t('workspace.useExistingFolder')}
                </DropdownMenu.Item>
              </DropdownMenu.SubContent>
            </DropdownMenu.Portal>
          </DropdownMenu.Sub>
        </DropdownMenu.Content>
      </DropdownMenu.Portal>
    </DropdownMenu.Root>
  )
}
