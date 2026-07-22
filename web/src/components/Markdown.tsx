import { useEffect, useMemo, useRef } from 'react'
import { Marked } from 'marked'
import hljs from 'highlight.js/lib/common'
import DOMPurify from 'dompurify'
import 'highlight.js/styles/github.css'
import { useI18n } from '@/i18n'

const COPY_ICON =
  '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>'
const CHECK_ICON =
  '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>'

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

// One Marked instance whose fenced code blocks render inside a titled frame with
// a copy button, mirroring the chrome of the tool-output cards. The button is
// left empty here and given its icon and localized labels after render, so the
// sanitizer never has to allow inline SVG.
const marked = new Marked({
  renderer: {
    code({ text, lang }) {
      const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
      const highlighted = hljs.highlight(text, { language }).value
      const label = language === 'plaintext' ? '' : language
      return (
        `<div class="or-codeblock not-prose">` +
        `<div class="or-codeblock-head">` +
        `<span class="or-codeblock-lang">${escapeHtml(label)}</span>` +
        `<button class="or-md-copy" type="button"></button>` +
        `</div>` +
        `<pre><code class="hljs language-${escapeHtml(language)}">${highlighted}</code></pre>` +
        `</div>`
      )
    },
  },
})

// Model output is untrusted, so every render is sanitized before it reaches the
// DOM. Rendered inside Tailwind Typography for polished prose defaults.
export function Markdown({ source }: { source: string }) {
  const { t } = useI18n()
  const ref = useRef<HTMLDivElement>(null)
  const html = useMemo(
    () => DOMPurify.sanitize(marked.parse(source, { async: false }) as string),
    [source],
  )

  const copyLabel = t('code.copy')
  const copiedLabel = t('code.copied')

  // Give each copy button its icon and localized label once the sanitized HTML
  // is in the DOM, re-running when content or locale changes.
  useEffect(() => {
    const root = ref.current
    if (!root) return
    root.querySelectorAll<HTMLButtonElement>('.or-md-copy').forEach((button) => {
      if (button.dataset.state !== 'copied') button.innerHTML = COPY_ICON
      button.title = button.dataset.state === 'copied' ? copiedLabel : copyLabel
      button.setAttribute('aria-label', button.title)
    })
  }, [html, copyLabel, copiedLabel])

  const handleClick = (event: React.MouseEvent<HTMLDivElement>) => {
    const button = (event.target as HTMLElement).closest<HTMLButtonElement>('.or-md-copy')
    if (!button) return
    const code = button.closest('.or-codeblock')?.querySelector('code')?.textContent ?? ''
    void navigator.clipboard
      .writeText(code)
      .then(() => {
        button.dataset.state = 'copied'
        button.innerHTML = CHECK_ICON
        button.title = copiedLabel
        button.setAttribute('aria-label', copiedLabel)
        window.setTimeout(() => {
          delete button.dataset.state
          button.innerHTML = COPY_ICON
          button.title = copyLabel
          button.setAttribute('aria-label', copyLabel)
        }, 1600)
      })
      .catch(() => {
        // Clipboard access can be unavailable in non-secure browser contexts.
      })
  }

  return (
    <div
      ref={ref}
      onClick={handleClick}
      className="or-code-theme prose prose-stone max-w-none text-[var(--chat-font-size)] leading-[1.65] prose-headings:font-semibold prose-headings:tracking-normal prose-h1:mt-5 prose-h1:mb-2 prose-h1:text-[1.25rem] prose-h1:leading-7 prose-h2:mt-4.5 prose-h2:mb-2 prose-h2:text-[1.125rem] prose-h2:leading-6 prose-h3:mt-4 prose-h3:mb-1.5 prose-h3:text-[1.0625rem] prose-h3:leading-6 prose-h4:mt-3.5 prose-h4:mb-1.5 prose-h4:text-[1rem] prose-h4:leading-6 prose-p:my-1.5 prose-a:text-blue-700 prose-a:decoration-blue-200 prose-a:underline-offset-3 prose-strong:font-semibold prose-code:rounded prose-code:border prose-code:border-stone-200 prose-code:bg-stone-100 prose-code:px-1 prose-code:py-0.5 prose-code:text-[0.86em] prose-code:font-medium prose-code:before:content-none prose-code:after:content-none [&_table]:block [&_table]:max-w-full [&_table]:overflow-x-auto [&_pre_code.hljs]:block [&_pre_code.hljs]:overflow-x-auto [&_pre_code.hljs]:bg-white [&_pre_code.hljs]:px-4 [&_pre_code.hljs]:py-3.5 [&_pre_code.hljs]:font-mono [&_pre_code.hljs]:text-[var(--code-font-size)] [&_pre_code.hljs]:leading-6"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
