import { useEffect, useRef, useState } from 'react'
import {
  Activity,
  Check,
  ChevronDown,
  CircleCheck,
  CircleX,
  LoaderCircle,
  TriangleAlert,
  X,
} from 'lucide-react'
import { Dialog, DropdownMenu } from 'radix-ui'
import { apiURL } from '@/api'
import { useI18n } from '@/i18n'
import { cn } from '@/lib/utils'
import { ProviderIcon } from '@/components/ProviderIdentity'
import { FixedThinkingStatus } from '@/components/FixedThinkingStatus'
import { ThinkingModeToggle } from '@/components/ThinkingModeToggle'
import { composerMenuTriggerClass } from '@/components/composerControlStyles'
import {
  isFixedThinking,
  isToggleThinking,
  thinkingLevelLabelKey,
  toggleThinkingLevel,
} from '@/modelThinking'
import type { ModelOption, ThinkingLevel } from '@/types'

type TestConnection = {
  id: string
  name: string
  baseURL: string
  official: boolean
}

type TestCredential = {
  id: string
  name: string
  apiKey: string
}

type ConnectionTestStatus =
  | 'success'
  | 'authentication_failed'
  | 'rate_limited'
  | 'timeout'
  | 'unreachable'
  | 'not_found'
  | 'provider_error'

type ConnectionTestResponse = {
  status: ConnectionTestStatus
  model: string
  modelName: string
  requestText: string
  thinkingLevel: ThinkingLevel
  thinkingText: string
  responseText: string
  stopReason?: string
  inputTokens: number
  outputTokens: number
  latencyMs: number
  providerStatus?: number
}

export function ProviderConnectionTestDialog({
  providerId,
  providerLabel,
  preferredModelId,
  connection,
  credential,
  onOpenChange,
}: {
  providerId: string
  providerLabel: string
  preferredModelId: string
  connection: TestConnection
  credential: TestCredential
  onOpenChange: (open: boolean) => void
}) {
  const { t } = useI18n()
  const [models, setModels] = useState<ModelOption[]>([])
  const [selectedModelId, setSelectedModelId] = useState('')
  const [selectedThinkingLevel, setSelectedThinkingLevel] = useState<ThinkingLevel>('off')
  const [loadingModels, setLoadingModels] = useState(true)
  const [testing, setTesting] = useState(false)
  const [result, setResult] = useState<ConnectionTestResponse>()
  const [error, setError] = useState('')
  const requestRef = useRef<AbortController | undefined>(undefined)

  useEffect(() => {
    const controller = new AbortController()
    const load = async () => {
      setLoadingModels(true)
      setError('')
      try {
        const response = await fetch(apiURL('/models?scope=catalog'), {
          cache: 'no-store',
          signal: controller.signal,
        })
        if (!response.ok) throw new Error(`HTTP ${response.status}`)
        const data = (await response.json()) as { models: ModelOption[] }
        const available = data.models.filter((model) => model.provider === providerId)
        const initialModel = available.find((model) => model.id === preferredModelId) ?? available[0]
        setModels(available)
        setSelectedModelId(initialModel?.id ?? '')
        setSelectedThinkingLevel(defaultTestThinkingLevel(initialModel))
      } catch (cause) {
        if (cause instanceof DOMException && cause.name === 'AbortError') return
        setError(t('providers.testModelsFailed'))
      } finally {
        if (!controller.signal.aborted) setLoadingModels(false)
      }
    }
    void load()
    return () => controller.abort()
  }, [preferredModelId, providerId, t])

  useEffect(() => () => requestRef.current?.abort(), [])

  const selectedModel = models.find((model) => model.id === selectedModelId)
  const fixedThinking = isFixedThinking(selectedModel)
  const toggleThinking = isToggleThinking(selectedModel)
  const connectionName = connection.official
    ? t('providers.officialConnection')
    : connection.name || t('providers.customConnection')
  const credentialName = credential.name || t('providers.newKey')

  const chooseModel = (modelId: string) => {
    const nextModel = models.find((model) => model.id === modelId)
    setSelectedModelId(modelId)
    setSelectedThinkingLevel((current) =>
      nextModel?.thinkingLevels.includes(current)
        ? current
        : defaultTestThinkingLevel(nextModel),
    )
    setResult(undefined)
    setError('')
  }

  const chooseThinkingLevel = (level: string) => {
    setSelectedThinkingLevel(level as ThinkingLevel)
    setResult(undefined)
    setError('')
  }

  const runTest = async () => {
    if (!selectedModelId || testing) return
    requestRef.current?.abort()
    const controller = new AbortController()
    requestRef.current = controller
    setTesting(true)
    setResult(undefined)
    setError('')
    try {
      const response = await fetch(apiURL(`/providers/${providerId}/test-connection`), {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        signal: controller.signal,
        body: JSON.stringify({
          connectionId: connection.id,
          keyId: credential.id,
          baseURL: connection.official ? '' : connection.baseURL,
          apiKey: credential.apiKey,
          model: selectedModelId,
          thinkingLevel: selectedThinkingLevel,
        }),
      })
      if (!response.ok) {
        const body = (await response.json().catch(() => ({}))) as { error?: string }
        throw new Error(body.error || `HTTP ${response.status}`)
      }
      setResult((await response.json()) as ConnectionTestResponse)
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === 'AbortError') return
      setError(cause instanceof Error && cause.message ? cause.message : t('providers.testFailed'))
    } finally {
      if (requestRef.current === controller) requestRef.current = undefined
      if (!controller.signal.aborted) setTesting(false)
    }
  }

  const resultLabel = result
    ? {
        success: result.responseText
          ? t('providers.testSuccess')
          : t('providers.testSuccessNoText'),
        authentication_failed: t('providers.testAuthenticationFailed'),
        rate_limited: t('providers.testRateLimited'),
        timeout: t('providers.testTimeout'),
        unreachable: t('providers.testUnreachable'),
        not_found: t('providers.testNotFound'),
        provider_error: t('providers.testProviderError'),
      }[result.status]
    : ''
  const resultTone =
    result?.status === 'success'
      ? result.responseText
        ? 'success'
        : 'warning'
      : result?.status === 'rate_limited'
        ? 'warning'
        : 'error'
  const ResultIcon =
    resultTone === 'success' ? CircleCheck : resultTone === 'warning' ? TriangleAlert : CircleX
  const resultColor =
    resultTone === 'success'
      ? 'text-emerald-700'
      : resultTone === 'warning'
        ? 'text-amber-700'
        : 'text-red-700'
  const requestText = result?.requestText || 'hi'

  return (
    <Dialog.Root open onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="fixed inset-0 z-[160] animate-[fade-in_120ms_ease-out] bg-black/25 backdrop-blur-[1px]" />
        <Dialog.Content className="fixed top-1/2 left-1/2 z-[170] flex max-h-[min(42rem,calc(100vh-2rem))] w-[min(30rem,calc(100vw-2rem))] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-[14px] border border-stone-200 bg-white shadow-[0_28px_80px_-32px_rgba(28,25,23,0.55)] outline-none">
          <div className="flex items-start gap-3 border-b border-stone-100 px-5 py-4">
            <div className="grid size-9 shrink-0 place-items-center rounded-lg bg-stone-100">
              <ProviderIcon provider={providerId} />
            </div>
            <div className="min-w-0 flex-1">
              <Dialog.Title className="text-[0.9375rem] font-medium text-stone-950">
                {t('providers.testConnection')}
              </Dialog.Title>
              <Dialog.Description className="sr-only">
                {t('providers.testDescription')}
              </Dialog.Description>
              <p className="mt-0.5 truncate text-[0.75rem] text-stone-500">
                {providerLabel} · {connectionName} · {credentialName}
              </p>
            </div>
            <Dialog.Close asChild>
              <button
                type="button"
                className="grid size-8 shrink-0 cursor-pointer place-items-center rounded-md text-stone-400 transition-colors hover:bg-stone-100 hover:text-stone-800"
                aria-label={t('common.close')}
              >
                <X className="size-4" aria-hidden="true" />
              </button>
            </Dialog.Close>
          </div>

          <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
            <div className="grid grid-cols-[minmax(0,1.65fr)_minmax(8.5rem,1fr)] gap-3 max-sm:grid-cols-1">
              <div className="min-w-0">
                <span className="mb-1.5 block text-[0.6875rem] font-medium text-stone-500">
                  {t('providers.testModel')}
                </span>
                <DropdownMenu.Root>
                  <DropdownMenu.Trigger asChild>
                    <button
                      type="button"
                      aria-label={t('providers.testModel')}
                      disabled={loadingModels || testing || models.length === 0}
                      className={cn(
                        composerMenuTriggerClass,
                        'w-full max-w-none justify-between bg-[rgb(246,246,246)]',
                      )}
                    >
                      {loadingModels ? (
                        <LoaderCircle
                          className="size-3.5 shrink-0 animate-spin"
                          aria-hidden="true"
                        />
                      ) : (
                        <ProviderIcon provider={providerId} />
                      )}
                      <span className="min-w-0 flex-1 truncate text-left">
                        {loadingModels
                          ? t('providers.loading')
                          : selectedModel?.name || t('model.select')}
                      </span>
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
                      sideOffset={7}
                      collisionPadding={10}
                      className="z-[190] max-h-[min(26.25rem,var(--radix-dropdown-menu-content-available-height))] min-w-[16.25rem] animate-[fade-in_110ms_ease-out] overflow-y-auto rounded-2xl border border-stone-200 bg-white p-1 text-[0.875rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                    >
                      <DropdownMenu.Label className="px-2.5 py-1.5 text-[0.75rem] font-medium text-stone-400">
                        {t('model.models', { provider: providerLabel })}
                      </DropdownMenu.Label>
                      <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-stone-100" />
                      <DropdownMenu.RadioGroup
                        className="flex flex-col gap-0.5"
                        value={selectedModelId}
                        onValueChange={chooseModel}
                      >
                        {models.map((model) => (
                          <DropdownMenu.RadioItem
                            key={model.id}
                            value={model.id}
                            className="relative flex h-[30px] cursor-default items-center gap-2 rounded-[10px] px-2.5 pr-9 outline-none select-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)] data-[state=checked]:font-medium"
                          >
                            <span className="min-w-0 flex-1 truncate">{model.name}</span>
                            <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-stone-700">
                              <Check className="size-3.5" aria-hidden="true" />
                            </DropdownMenu.ItemIndicator>
                          </DropdownMenu.RadioItem>
                        ))}
                      </DropdownMenu.RadioGroup>
                    </DropdownMenu.Content>
                  </DropdownMenu.Portal>
                </DropdownMenu.Root>
              </div>

              <div className="min-w-0">
                <span className="mb-1.5 block text-[0.6875rem] font-medium text-stone-500">
                  {t('providers.testThinking')}
                </span>
                {fixedThinking ? (
                  <FixedThinkingStatus
                    className="h-9 w-full justify-center rounded-xl bg-[rgb(246,246,246)] px-2.5 text-[0.8125rem] text-stone-500 outline-none hover:bg-[rgb(241,241,241)] focus-visible:bg-[rgb(241,241,241)]"
                    hidden={selectedModel?.thinkingVisibility === 'hidden'}
                  />
                ) : toggleThinking ? (
                  <ThinkingModeToggle
                    checked={selectedThinkingLevel === 'high'}
                    disabled={testing}
                    ariaLabel={t('providers.testThinking')}
                    className="w-full justify-between bg-[rgb(246,246,246)]"
                    onCheckedChange={(checked) => {
                      chooseThinkingLevel(toggleThinkingLevel(checked))
                    }}
                  />
                ) : (
                  <DropdownMenu.Root>
                    <DropdownMenu.Trigger asChild>
                      <button
                        type="button"
                        aria-label={t('providers.testThinking')}
                        disabled={
                          testing ||
                          !selectedModel ||
                          selectedModel.thinkingLevels.length === 0
                        }
                        className={cn(
                          composerMenuTriggerClass,
                          'w-full justify-between bg-[rgb(246,246,246)]',
                        )}
                      >
                        <span className="truncate">
                          {t(`effort.${selectedThinkingLevel}`)}
                        </span>
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
                        sideOffset={7}
                        collisionPadding={10}
                        className="z-[190] min-w-[11rem] animate-[fade-in_110ms_ease-out] rounded-2xl border border-stone-200 bg-white p-1 text-[0.875rem] text-stone-900 shadow-[0_16px_44px_-24px_rgba(28,25,23,0.48)] outline-none"
                      >
                        <DropdownMenu.Label className="px-2.5 py-1.5 text-[0.75rem] font-medium text-stone-400">
                          {t('providers.testThinking')}
                        </DropdownMenu.Label>
                        <DropdownMenu.Separator className="mx-1.5 my-0.5 h-px bg-stone-100" />
                        <DropdownMenu.RadioGroup
                          className="flex flex-col gap-0.5"
                          value={selectedThinkingLevel}
                          onValueChange={chooseThinkingLevel}
                        >
                          {(selectedModel?.thinkingLevels ?? []).map((level) => (
                            <DropdownMenu.RadioItem
                              key={level}
                              value={level}
                              className="relative flex h-[30px] cursor-default items-center rounded-[10px] px-2.5 pr-9 outline-none select-none data-[highlighted]:bg-[rgb(241,241,241)] data-[state=checked]:bg-[rgb(237,237,237)] data-[state=checked]:font-medium"
                            >
                              <span>{t(`effort.${level}`)}</span>
                              <DropdownMenu.ItemIndicator className="absolute right-2.5 grid size-4 place-items-center text-stone-700">
                                <Check className="size-3.5" aria-hidden="true" />
                              </DropdownMenu.ItemIndicator>
                            </DropdownMenu.RadioItem>
                          ))}
                        </DropdownMenu.RadioGroup>
                      </DropdownMenu.Content>
                    </DropdownMenu.Portal>
                  </DropdownMenu.Root>
                )}
              </div>
            </div>

            <div className="mt-4">
              <div className="text-[0.6875rem] font-medium text-stone-400">
                {t('providers.testRequest')}
              </div>
              <div className="mt-1.5 min-h-9 rounded-md border border-stone-200 bg-stone-50 px-3 py-2 font-mono text-[0.75rem] leading-[1.125rem] text-stone-700">
                {requestText}
              </div>
            </div>

            {testing && (
              <div
                className="mt-4 flex items-center gap-2 text-[0.75rem] text-stone-500"
                aria-live="polite"
              >
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
                {t('providers.testing')}
              </div>
            )}

            {error && !result && (
              <div
                aria-live="polite"
                className="mt-4 flex items-start gap-2.5 rounded-lg border border-red-200 bg-red-50/65 px-3 py-2.5 text-red-700"
              >
                <CircleX className="mt-0.5 size-4 shrink-0" aria-hidden="true" />
                <p className="min-w-0 flex-1 text-[0.78125rem] font-medium">{error}</p>
              </div>
            )}

            {result && (
              <div
                className="mt-4 overflow-hidden rounded-lg border border-stone-200 bg-white"
                aria-live="polite"
              >
                <div className="flex items-start gap-2.5 px-3.5 py-3">
                  <ResultIcon
                    className={cn('mt-0.5 size-4 shrink-0', resultColor)}
                    aria-hidden="true"
                  />
                  <div className="min-w-0 flex-1">
                    <p className={cn('text-[0.78125rem] font-medium', resultColor)}>
                      {resultLabel}
                    </p>
                    <p className="mt-0.5 truncate text-[0.71875rem] text-stone-400">
                      {result.modelName || selectedModel?.name || result.model}
                      {' · '}
                      {t(thinkingLevelLabelKey(selectedModel, result.thinkingLevel))}
                      {' · '}
                      {t('providers.testTokens', {
                        input: result.inputTokens,
                        output: result.outputTokens,
                      })}
                      {result.stopReason
                        ? ` · ${t('providers.testStopReason', { reason: result.stopReason })}`
                        : ''}
                      {' · '}
                      {result.latencyMs} ms
                      {result.providerStatus ? ` · HTTP ${result.providerStatus}` : ''}
                    </p>
                  </div>
                </div>
                <div className="border-t border-stone-100 bg-stone-50/55 px-3.5 py-3">
                  <div className="text-[0.6875rem] font-medium text-stone-400">
                    {t('providers.testThinkingOutput')}
                  </div>
                  <p
                    className={cn(
                      'mt-1 max-h-44 overflow-y-auto whitespace-pre-wrap break-words font-mono text-[0.75rem] leading-5',
                      result.thinkingText ? 'text-stone-600' : 'text-stone-400',
                    )}
                  >
                    {result.thinkingText || t('providers.testNoThinking')}
                  </p>
                </div>
                <div className="border-t border-stone-100 bg-stone-50/55 px-3.5 py-3">
                  <div className="text-[0.6875rem] font-medium text-stone-400">
                    {t('providers.testResponse')}
                  </div>
                  <p
                    className={cn(
                      'mt-1 whitespace-pre-wrap break-words font-mono text-[0.75rem] leading-5',
                      result.responseText ? 'text-stone-700' : 'text-stone-400',
                    )}
                  >
                    {result.responseText || t('providers.testNoResponse')}
                  </p>
                </div>
              </div>
            )}
          </div>

          <div className="flex items-center justify-end gap-2 border-t border-stone-100 px-5 py-3.5">
            <Dialog.Close asChild>
              <button
                type="button"
                className="h-8 cursor-pointer rounded-md px-3 text-[0.75rem] font-medium text-stone-500 transition-colors hover:bg-stone-100 hover:text-stone-900"
              >
                {t('common.close')}
              </button>
            </Dialog.Close>
            <button
              type="button"
              onClick={() => void runTest()}
              disabled={!selectedModelId || loadingModels || testing}
              className="inline-flex h-8 min-w-[6.5rem] cursor-pointer items-center justify-center gap-1.5 rounded-md bg-stone-950 px-3 text-[0.75rem] font-medium text-white transition-colors hover:bg-stone-800 disabled:cursor-not-allowed disabled:bg-stone-100 disabled:text-stone-400"
            >
              {testing ? (
                <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
              ) : (
                <Activity className="size-3.5" aria-hidden="true" />
              )}
              {testing
                ? t('providers.testing')
                : result
                  ? t('providers.retest')
                  : t('providers.testConnection')}
            </button>
          </div>
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  )
}

function defaultTestThinkingLevel(model?: ModelOption): ThinkingLevel {
  if (!model || model.thinkingLevels.length === 0) return 'off'
  if (model.thinkingLevels.includes('off')) return 'off'
  return model.thinkingLevels[0]
}
