import {
  Check,
  ChevronRight,
  Gauge,
  Ghost,
  Languages,
  LogOut,
  Send,
  Settings,
  type LucideIcon,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import avatarImage from '@/assets/avatar.jpg'
import { cn } from '@/lib/utils'
import { useI18n, type Locale } from '@/i18n'

export function ProfileMenu({
  collapsed,
  onOpenUsage,
  onOpenSettings,
}: {
  collapsed: boolean
  onOpenUsage: () => void
  onOpenSettings: () => void
}) {
  const { locale, setLocale, t } = useI18n()

  return (
    <div className="w-full shrink-0 border-t border-stone-200/70 p-3 max-md:w-[17.5rem]">
      <div className="flex items-center gap-2">
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              className={cn(
                'flex h-8 cursor-pointer items-center overflow-hidden outline-none transition-colors hover:bg-[rgb(246,246,246)] focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(237,237,237)]',
                collapsed
                  ? 'w-8 flex-none justify-center rounded-full p-0.5'
                  : 'min-w-0 flex-1 gap-2.5 rounded-[10px] px-2.5 text-left',
              )}
              type="button"
              aria-label={t('profile.openMenu')}
            >
              <Avatar />
              <span
                className={cn(
                  'min-w-0 truncate whitespace-nowrap text-[0.875rem] font-medium text-stone-900 transition-opacity duration-100 ease-out motion-reduce:transition-none',
                  collapsed ? 'w-0 opacity-0' : 'opacity-100',
                )}
                aria-hidden={collapsed}
              >
                Ktsoator
              </span>
            </button>
          </DropdownMenu.Trigger>

          <DropdownMenu.Portal>
            <DropdownMenu.Content
              side="top"
              align="start"
              sideOffset={7}
              collisionPadding={10}
              className="z-[120] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[0.875rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              style={{
                width: collapsed ? '14.5rem' : 'var(--radix-dropdown-menu-trigger-width)',
              }}
            >
              <DropdownMenu.Label className="flex h-9 items-center gap-2.5 px-2.5">
                <Avatar />
                <span className="truncate font-medium">Ktsoator</span>
              </DropdownMenu.Label>
              <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-stone-100" />

              <ProfileItem
                icon={Gauge}
                label={t('profile.usageRemaining')}
                trailing="chevron"
                onSelect={onOpenUsage}
              />
              <ProfileItem icon={Ghost} label={t('profile.showPet')} />
              <ProfileItem icon={Send} label={t('profile.inviteFriend')} />
              <ProfileItem
                icon={Settings}
                label={t('profile.settings')}
                shortcut="⌘,"
                onSelect={onOpenSettings}
              />

              <DropdownMenu.Sub>
                <DropdownMenu.SubTrigger className={profileItemClass}>
                  <Languages className="size-[1.0625rem] shrink-0 text-stone-600" aria-hidden="true" />
                  <span className="min-w-0 flex-1 truncate">{t('profile.language')}</span>
                  <span className="text-[0.75rem] text-stone-400">
                    {locale === 'zh-CN' ? t('profile.chinese') : t('profile.english')}
                  </span>
                  <ChevronRight className="size-4 shrink-0 text-stone-400" aria-hidden="true" />
                </DropdownMenu.SubTrigger>
                <DropdownMenu.Portal>
                  <DropdownMenu.SubContent
                    sideOffset={6}
                    alignOffset={-4}
                    collisionPadding={10}
                    className="z-[130] min-w-[11.5rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[0.875rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                  >
                    <DropdownMenu.Label className="px-2.5 py-1.5 text-[0.75rem] font-medium text-stone-400">
                      {t('profile.language')}
                    </DropdownMenu.Label>
                    <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-stone-100" />
                    <DropdownMenu.RadioGroup
                      className="flex flex-col gap-0.5"
                      value={locale}
                      onValueChange={(value) => setLocale(value as Locale)}
                    >
                      <LanguageItem value="en" label={t('profile.english')} />
                      <LanguageItem value="zh-CN" label={t('profile.chinese')} />
                    </DropdownMenu.RadioGroup>
                  </DropdownMenu.SubContent>
                </DropdownMenu.Portal>
              </DropdownMenu.Sub>

              <ProfileItem icon={LogOut} label={t('profile.logOut')} />
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>

      </div>
    </div>
  )
}

function Avatar() {
  return (
    <img
      className="size-7 shrink-0 rounded-full border border-stone-200 object-cover shadow-sm"
      src={avatarImage}
      alt=""
      aria-hidden="true"
    />
  )
}

function ProfileItem({
  icon: Icon,
  label,
  shortcut,
  trailing,
  onSelect,
}: {
  icon: LucideIcon
  label: string
  shortcut?: string
  trailing?: 'chevron'
  onSelect?: () => void
}) {
  return (
    <DropdownMenu.Item className={profileItemClass} onSelect={onSelect}>
      <Icon className="size-[1.0625rem] shrink-0 text-stone-600" aria-hidden="true" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {shortcut && <span className="text-[0.75rem] text-stone-400">{shortcut}</span>}
      {trailing === 'chevron' && (
        <ChevronRight className="size-4 shrink-0 text-stone-400" aria-hidden="true" />
      )}
    </DropdownMenu.Item>
  )
}

function LanguageItem({ value, label }: { value: Locale; label: string }) {
  return (
    <DropdownMenu.RadioItem
      value={value}
      className="relative flex h-9 cursor-default select-none items-center rounded-[10px] px-2.5 pr-8 outline-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)]"
    >
      <span>{label}</span>
      <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-stone-700">
        <Check className="size-3.5" aria-hidden="true" />
      </DropdownMenu.ItemIndicator>
    </DropdownMenu.RadioItem>
  )
}

const profileItemClass =
  'relative mb-0.5 flex h-9 cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none last:mb-0 data-[highlighted]:bg-[rgb(241,241,241)] data-[state=open]:bg-[rgb(237,237,237)]'
