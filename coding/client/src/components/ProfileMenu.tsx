import {
  BookOpenText,
  Check,
  ChevronRight,
  Gauge,
  Languages,
  LogOut,
  Megaphone,
  Monitor,
  Moon,
  Settings,
  Sun,
  type LucideIcon,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import avatarImage from '@/assets/avatar.jpg'
import { cn } from '@/lib/utils'
import { useI18n, type Locale } from '@/i18n'
import { useTheme } from '@/useTheme'
import type { ThemePreference } from '@/theme'

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
  const { preference: theme, setPreference: setTheme } = useTheme()

  return (
    <div className="w-full shrink-0 border-t border-edge/70 px-3 py-2 max-md:w-[17.5rem]">
      <div className="flex items-center gap-2">
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              className={cn(
                'flex h-8 cursor-pointer items-center overflow-hidden outline-none transition-colors hover:bg-surface-hover focus-visible:bg-surface-hover data-[state=open]:bg-surface-selected',
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
                  'min-w-0 truncate whitespace-nowrap text-[0.875rem] font-medium text-ink transition-opacity duration-100 ease-out motion-reduce:transition-none',
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
              className="z-[120] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1 text-[0.875rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
              style={{
                width: collapsed ? '14.5rem' : 'var(--radix-dropdown-menu-trigger-width)',
              }}
            >
              <DropdownMenu.Label className="flex h-9 items-center gap-2.5 px-2.5">
                <Avatar />
                <span className="truncate font-medium">Ktsoator</span>
              </DropdownMenu.Label>
              <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-canvas-sunken" />

              <ProfileItem
                icon={Gauge}
                label={t('profile.usageRemaining')}
                trailing="chevron"
                onSelect={onOpenUsage}
              />
              <ProfileItem icon={BookOpenText} label={t('profile.documentation')} />
              <ProfileItem icon={Megaphone} label={t('profile.changelog')} />
              <ProfileItem
                icon={Settings}
                label={t('profile.settings')}
                shortcut="⌘,"
                onSelect={onOpenSettings}
              />

              <DropdownMenu.Sub>
                <DropdownMenu.SubTrigger className={profileItemClass}>
                  <ThemeIcon
                    preference={theme}
                    className="size-[1.0625rem] shrink-0 text-ink-muted"
                  />
                  <span className="min-w-0 flex-1 truncate">{t('profile.theme')}</span>
                  <span className="text-[0.75rem] text-ink-faint">{t(themeLabelKey(theme))}</span>
                  <ChevronRight className="size-4 shrink-0 text-ink-faint" aria-hidden="true" />
                </DropdownMenu.SubTrigger>
                <DropdownMenu.Portal>
                  <DropdownMenu.SubContent
                    sideOffset={6}
                    alignOffset={-4}
                    collisionPadding={10}
                    className="z-[130] min-w-[11.5rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1 text-[0.875rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                  >
                    <DropdownMenu.Label className="px-2.5 py-1.5 text-[0.75rem] font-medium text-ink-faint">
                      {t('profile.theme')}
                    </DropdownMenu.Label>
                    <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-canvas-sunken" />
                    <DropdownMenu.RadioGroup
                      className="flex flex-col gap-0.5"
                      value={theme}
                      onValueChange={(value) => setTheme(value as ThemePreference)}
                    >
                      <ChoiceItem value="system" label={t('profile.themeSystem')} />
                      <ChoiceItem value="light" label={t('profile.themeLight')} />
                      <ChoiceItem value="dark" label={t('profile.themeDark')} />
                    </DropdownMenu.RadioGroup>
                  </DropdownMenu.SubContent>
                </DropdownMenu.Portal>
              </DropdownMenu.Sub>

              <DropdownMenu.Sub>
                <DropdownMenu.SubTrigger className={profileItemClass}>
                  <Languages className="size-[1.0625rem] shrink-0 text-ink-muted" aria-hidden="true" />
                  <span className="min-w-0 flex-1 truncate">{t('profile.language')}</span>
                  <span className="text-[0.75rem] text-ink-faint">
                    {locale === 'zh-CN' ? t('profile.chinese') : t('profile.english')}
                  </span>
                  <ChevronRight className="size-4 shrink-0 text-ink-faint" aria-hidden="true" />
                </DropdownMenu.SubTrigger>
                <DropdownMenu.Portal>
                  <DropdownMenu.SubContent
                    sideOffset={6}
                    alignOffset={-4}
                    collisionPadding={10}
                    className="z-[130] min-w-[11.5rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-edge bg-canvas p-1 text-[0.875rem] text-ink shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                  >
                    <DropdownMenu.Label className="px-2.5 py-1.5 text-[0.75rem] font-medium text-ink-faint">
                      {t('profile.language')}
                    </DropdownMenu.Label>
                    <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-canvas-sunken" />
                    <DropdownMenu.RadioGroup
                      className="flex flex-col gap-0.5"
                      value={locale}
                      onValueChange={(value) => setLocale(value as Locale)}
                    >
                      <ChoiceItem value="en" label={t('profile.english')} />
                      <ChoiceItem value="zh-CN" label={t('profile.chinese')} />
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
      className="size-7 shrink-0 rounded-full border border-edge object-cover shadow-sm"
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
      <Icon className="size-[1.0625rem] shrink-0 text-ink-muted" aria-hidden="true" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {shortcut && <span className="text-[0.75rem] text-ink-faint">{shortcut}</span>}
      {trailing === 'chevron' && (
        <ChevronRight className="size-4 shrink-0 text-ink-faint" aria-hidden="true" />
      )}
    </DropdownMenu.Item>
  )
}

function ThemeIcon({
  preference,
  className,
}: {
  preference: ThemePreference
  className: string
}) {
  const Icon = preference === 'dark' ? Moon : preference === 'light' ? Sun : Monitor
  return <Icon className={className} aria-hidden="true" />
}

function themeLabelKey(preference: ThemePreference) {
  if (preference === 'light') return 'profile.themeLight' as const
  if (preference === 'dark') return 'profile.themeDark' as const
  return 'profile.themeSystem' as const
}

function ChoiceItem({ value, label }: { value: Locale | ThemePreference; label: string }) {
  return (
    <DropdownMenu.RadioItem
      value={value}
      className="relative flex h-9 cursor-default select-none items-center rounded-[10px] px-2.5 pr-8 outline-none data-[highlighted]:bg-surface-active data-[state=checked]:bg-surface-selected"
    >
      <span>{label}</span>
      <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-ink-soft">
        <Check className="size-3.5" aria-hidden="true" />
      </DropdownMenu.ItemIndicator>
    </DropdownMenu.RadioItem>
  )
}

const profileItemClass =
  'relative mb-0.5 flex h-9 cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none last:mb-0 data-[highlighted]:bg-surface-active data-[state=open]:bg-surface-selected'
