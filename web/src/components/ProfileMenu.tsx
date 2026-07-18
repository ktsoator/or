import { useState } from 'react'
import {
  ChevronRight,
  CircleHelp,
  Gauge,
  Ghost,
  LogOut,
  Send,
  Settings,
  type LucideIcon,
} from 'lucide-react'
import { DropdownMenu } from 'radix-ui'
import avatarImage from '@/assets/avatar.jpg'
import { cn } from '@/lib/utils'

export function ProfileMenu({ collapsed }: { collapsed: boolean }) {
  const [helpActive, setHelpActive] = useState(false)

  return (
    <div className="w-[256px] shrink-0 border-t border-stone-200/70 p-3 max-md:w-[280px]">
      <div className="flex items-center gap-2">
        <DropdownMenu.Root>
          <DropdownMenu.Trigger asChild>
            <button
              className={cn(
                'flex h-9 cursor-pointer items-center overflow-hidden outline-none transition-colors hover:bg-[rgb(241,241,241)] focus-visible:ring-2 focus-visible:ring-stone-300 data-[state=open]:bg-[rgb(241,241,241)]',
                collapsed
                  ? 'ml-0.5 w-9 flex-none justify-center rounded-full p-1'
                  : 'min-w-0 flex-1 gap-2.5 rounded-[10px] px-2 text-left',
              )}
              type="button"
              aria-label="Open profile menu"
            >
              <Avatar />
              <span
                className={cn(
                  'min-w-0 truncate whitespace-nowrap text-[14px] font-[560] text-stone-900 transition-opacity duration-100 ease-out motion-reduce:transition-none',
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
              className="z-[120] w-[232px] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[14px] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
            >
              <DropdownMenu.Label className="flex h-9 items-center gap-2.5 px-2.5">
                <Avatar />
                <span className="truncate font-[600]">Ktsoator</span>
              </DropdownMenu.Label>
              <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-stone-100" />

              <ProfileItem icon={Gauge} label="Usage remaining" trailing="chevron" />
              <ProfileItem icon={Ghost} label="Show pet" />
              <ProfileItem icon={Send} label="Invite a friend" />
              <ProfileItem icon={Settings} label="Settings" shortcut="⌘," />
              <ProfileItem icon={LogOut} label="Log out" />
            </DropdownMenu.Content>
          </DropdownMenu.Portal>
        </DropdownMenu.Root>

        <button
          className={cn(
            'grid size-8 shrink-0 cursor-pointer place-items-center rounded-full text-stone-500 outline-none transition-colors hover:bg-[rgb(241,241,241)] hover:text-stone-800 focus-visible:ring-2 focus-visible:ring-stone-300',
            helpActive && 'bg-[rgb(241,241,241)] text-stone-800',
            collapsed && 'pointer-events-none opacity-0',
          )}
          type="button"
          title="Help"
          aria-label="Help"
          aria-pressed={helpActive}
          onClick={() => setHelpActive((active) => !active)}
        >
          <CircleHelp className="size-[17px]" aria-hidden="true" />
        </button>
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
}: {
  icon: LucideIcon
  label: string
  shortcut?: string
  trailing?: 'chevron'
}) {
  return (
    <DropdownMenu.Item className="relative flex h-9 cursor-default select-none items-center gap-2.5 rounded-[10px] px-2.5 outline-none data-[highlighted]:bg-[rgb(241,241,241)]">
      <Icon className="size-[17px] shrink-0 text-stone-600" aria-hidden="true" />
      <span className="min-w-0 flex-1 truncate">{label}</span>
      {shortcut && <span className="text-[12px] text-stone-400">{shortcut}</span>}
      {trailing === 'chevron' && (
        <ChevronRight className="size-4 shrink-0 text-stone-400" aria-hidden="true" />
      )}
    </DropdownMenu.Item>
  )
}
