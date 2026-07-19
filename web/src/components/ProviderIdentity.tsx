import { cn } from '@/lib/utils'
import deepseekIcon from '@/assets/providers/deepseek.svg'
import kimiIcon from '@/assets/providers/kimi.svg'
import minimaxIcon from '@/assets/providers/minimax.svg'
import xiaomiMimoIcon from '@/assets/providers/xiaomi-mimo.svg'
import zaiIcon from '@/assets/providers/zai.svg'
import { providerName } from '@/lib/provider'

const providerIcons: Record<string, string> = {
  deepseek: deepseekIcon,
  minimax: minimaxIcon,
  'minimax-cn': minimaxIcon,
  'kimi-coding': kimiIcon,
  moonshotai: kimiIcon,
  'moonshotai-cn': kimiIcon,
  xiaomi: xiaomiMimoIcon,
  'xiaomi-token-plan-ams': xiaomiMimoIcon,
  'xiaomi-token-plan-cn': xiaomiMimoIcon,
  'xiaomi-token-plan-sgp': xiaomiMimoIcon,
  zai: zaiIcon,
  'zai-coding-cn': zaiIcon,
}

export function ProviderIcon({ provider }: { provider: string }) {
  const source = providerIcons[provider]
  const kimi = source === kimiIcon

  if (!source) {
    return (
      <span
        className="grid size-[1.0625rem] shrink-0 place-items-center rounded-[5px] bg-stone-100 text-[0.5625rem] font-semibold text-stone-500"
        aria-hidden="true"
      >
        {providerName(provider).charAt(0) || '·'}
      </span>
    )
  }

  return (
    <span
      className={cn(
        'grid size-[1.0625rem] shrink-0 place-items-center overflow-hidden',
        kimi && 'rounded-[5px] bg-[#1783ff] p-[2px]',
      )}
      aria-hidden="true"
    >
      <img className="size-full object-contain" src={source} alt="" />
    </span>
  )
}
