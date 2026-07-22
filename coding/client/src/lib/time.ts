export function formatMessageTime(value: string, locale: string): string {
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return ''

  return new Intl.DateTimeFormat(locale === 'zh-CN' ? 'zh-CN' : 'en-US', {
    hour: 'numeric',
    minute: '2-digit',
  }).format(date)
}
