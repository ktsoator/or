import { cn } from '@/lib/utils'
import anthropicDark from '@/assets/providers/anthropic-dark.png'
import anthropicLight from '@/assets/providers/anthropic-light.png'
import cerebrasDark from '@/assets/providers/cerebras-dark.png'
import cerebrasLight from '@/assets/providers/cerebras-light.png'
import deepseekIcon from '@/assets/providers/deepseek.png'
import fireworksIcon from '@/assets/providers/fireworks.png'
import githubCopilotDark from '@/assets/providers/github-copilot-dark.png'
import githubCopilotLight from '@/assets/providers/github-copilot-light.png'
import googleIcon from '@/assets/providers/google.svg'
import groqDark from '@/assets/providers/groq-dark.png'
import groqLight from '@/assets/providers/groq-light.png'
import huggingFaceIcon from '@/assets/providers/huggingface.png'
import kimiIcon from '@/assets/providers/kimi.png'
import minimaxIcon from '@/assets/providers/minimax.png'
import mistralIcon from '@/assets/providers/mistral.svg'
import nvidiaIcon from '@/assets/providers/nvidia.png'
import openAIIcon from '@/assets/providers/openai.svg'
import openCodeDark from '@/assets/providers/opencode-dark.png'
import openCodeLight from '@/assets/providers/opencode-light.png'
import togetherDark from '@/assets/providers/together-dark.png'
import togetherLight from '@/assets/providers/together-light.png'
import xiaomiMimoDark from '@/assets/providers/xiaomi-mimo-dark.png'
import xiaomiMimoLight from '@/assets/providers/xiaomi-mimo-light.png'
import xAIDark from '@/assets/providers/xai-dark.png'
import xAILight from '@/assets/providers/xai-light.png'
import zaiDark from '@/assets/providers/zai-dark.png'
import zaiLight from '@/assets/providers/zai-light.png'
import { providerName } from '@/lib/provider'

// A mark is either one image that reads on any background — a brand colour, or a
// light mark that carries its own backdrop — or a pair cut for each theme.
type ProviderMark = { src: string } | { light: string; dark: string }

const anthropicMark = { light: anthropicLight, dark: anthropicDark }
const cerebrasMark = { light: cerebrasLight, dark: cerebrasDark }
const githubCopilotMark = { light: githubCopilotLight, dark: githubCopilotDark }
const groqMark = { light: groqLight, dark: groqDark }
const openCodeMark = { light: openCodeLight, dark: openCodeDark }
const togetherMark = { light: togetherLight, dark: togetherDark }
const xAIMark = { light: xAILight, dark: xAIDark }
const xiaomiMimoMark = { light: xiaomiMimoLight, dark: xiaomiMimoDark }
const zaiMark = { light: zaiLight, dark: zaiDark }

const deepseekMark = { src: deepseekIcon }
const fireworksMark = { src: fireworksIcon }
const googleMark = { src: googleIcon }
const huggingFaceMark = { src: huggingFaceIcon }
// The Kimi mark is white with a blue accent, drawn to sit on its brand blue. It
// needs that backdrop in both themes, not a per-theme cut.
const kimiMark = { src: kimiIcon }
const minimaxMark = { src: minimaxIcon }
const mistralMark = { src: mistralIcon }
const nvidiaMark = { src: nvidiaIcon }
const openAIMark = { src: openAIIcon }

const providerMarks: Record<string, ProviderMark> = {
  anthropic: anthropicMark,
  cerebras: cerebrasMark,
  deepseek: deepseekMark,
  fireworks: fireworksMark,
  'github-copilot': githubCopilotMark,
  google: googleMark,
  groq: groqMark,
  huggingface: huggingFaceMark,
  minimax: minimaxMark,
  'minimax-cn': minimaxMark,
  'kimi-coding': kimiMark,
  mistral: mistralMark,
  moonshotai: kimiMark,
  'moonshotai-cn': kimiMark,
  nvidia: nvidiaMark,
  openai: openAIMark,
  opencode: openCodeMark,
  'opencode-go': openCodeMark,
  together: togetherMark,
  xai: xAIMark,
  xiaomi: xiaomiMimoMark,
  'xiaomi-token-plan-ams': xiaomiMimoMark,
  'xiaomi-token-plan-cn': xiaomiMimoMark,
  'xiaomi-token-plan-sgp': xiaomiMimoMark,
  zai: zaiMark,
  'zai-coding-cn': zaiMark,
}

export function ProviderIcon({ provider }: { provider: string }) {
  const mark = providerMarks[provider]

  if (!mark) {
    return (
      <span
        className="grid size-[1.0625rem] shrink-0 place-items-center rounded-[5px] bg-canvas-sunken text-[0.5625rem] font-semibold text-ink-muted"
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
        mark === kimiMark && 'rounded-[5px] bg-[#1783ff] p-[2px]',
      )}
      aria-hidden="true"
    >
      {'src' in mark ? (
        <img className="size-full object-contain" src={mark.src} alt="" />
      ) : (
        <>
          {/* Both cuts are rendered and CSS hides one, so the visible mark
              follows the theme without this component subscribing to it. */}
          <img
            className="theme-light-only size-full object-contain"
            src={mark.light}
            alt=""
          />
          <img className="theme-dark-only size-full object-contain" src={mark.dark} alt="" />
        </>
      )}
    </span>
  )
}
