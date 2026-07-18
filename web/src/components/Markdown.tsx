import { useMemo } from 'react'
import { Marked } from 'marked'
import { markedHighlight } from 'marked-highlight'
import hljs from 'highlight.js/lib/common'
import DOMPurify from 'dompurify'
import 'highlight.js/styles/github.css'

// One Marked instance with syntax highlighting for fenced code blocks.
const marked = new Marked(
  markedHighlight({
    emptyLangClass: 'hljs',
    langPrefix: 'hljs language-',
    highlight(code, lang) {
      const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext'
      return hljs.highlight(code, { language }).value
    },
  }),
)

// Model output is untrusted, so every render is sanitized before it reaches the
// DOM. Rendered inside Tailwind Typography for polished prose defaults.
export function Markdown({ source }: { source: string }) {
  const html = useMemo(
    () => DOMPurify.sanitize(marked.parse(source, { async: false }) as string),
    [source],
  )
  return (
    <div
      className="or-code-theme prose prose-stone max-w-none text-[16.5px] leading-[1.7] prose-headings:tracking-[-0.02em] prose-p:my-2 prose-a:text-blue-700 prose-a:decoration-blue-200 prose-a:underline-offset-3 prose-strong:font-semibold prose-pre:my-3 prose-pre:overflow-hidden prose-pre:rounded-lg prose-pre:border prose-pre:border-stone-200 prose-pre:bg-white prose-pre:p-0 prose-pre:shadow-none prose-code:rounded prose-code:border prose-code:border-stone-200 prose-code:bg-stone-100 prose-code:px-1 prose-code:py-0.5 prose-code:text-[0.86em] prose-code:font-medium prose-code:before:content-none prose-code:after:content-none [&_pre_code.hljs]:block [&_pre_code.hljs]:overflow-x-auto [&_pre_code.hljs]:bg-white [&_pre_code.hljs]:px-4 [&_pre_code.hljs]:py-3.5 [&_pre_code.hljs]:font-mono [&_pre_code.hljs]:text-[14px] [&_pre_code.hljs]:leading-6"
      dangerouslySetInnerHTML={{ __html: html }}
    />
  )
}
