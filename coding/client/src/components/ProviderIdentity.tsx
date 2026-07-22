import { cn } from '@/lib/utils'
import anthropicIcon from '@/assets/providers/anthropic.svg'
import cerebrasIcon from '@/assets/providers/cerebras.svg'
import deepseekIcon from '@/assets/providers/deepseek.svg'
import fireworksIcon from '@/assets/providers/fireworks.svg'
import githubCopilotIcon from '@/assets/providers/github-copilot.svg'
import googleIcon from '@/assets/providers/google.svg'
import groqIcon from '@/assets/providers/groq.svg'
import huggingFaceIcon from '@/assets/providers/huggingface.svg'
import kimiIcon from '@/assets/providers/kimi.svg'
import minimaxIcon from '@/assets/providers/minimax.svg'
import mistralIcon from '@/assets/providers/mistral.svg'
import nvidiaIcon from '@/assets/providers/nvidia.svg'
import openAIIcon from '@/assets/providers/openai.svg'
import openCodeIcon from '@/assets/providers/opencode.svg'
import openRouterIcon from '@/assets/providers/openrouter.svg'
import togetherIcon from '@/assets/providers/together.svg'
import vercelIcon from '@/assets/providers/vercel.svg'
import xiaomiMimoIcon from '@/assets/providers/xiaomi-mimo.svg'
import xAIIcon from '@/assets/providers/xai.svg'
import zaiIcon from '@/assets/providers/zai.svg'
import { providerName } from '@/lib/provider'

const providerIcons: Record<string, string> = {
  anthropic: anthropicIcon,
  cerebras: cerebrasIcon,
  deepseek: deepseekIcon,
  fireworks: fireworksIcon,
  'github-copilot': githubCopilotIcon,
  google: googleIcon,
  groq: groqIcon,
  huggingface: huggingFaceIcon,
  minimax: minimaxIcon,
  'minimax-cn': minimaxIcon,
  'kimi-coding': kimiIcon,
  mistral: mistralIcon,
  moonshotai: kimiIcon,
  'moonshotai-cn': kimiIcon,
  nvidia: nvidiaIcon,
  openai: openAIIcon,
  opencode: openCodeIcon,
  'opencode-go': openCodeIcon,
  openrouter: openRouterIcon,
  together: togetherIcon,
  'vercel-ai-gateway': vercelIcon,
  xai: xAIIcon,
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
