export function formatMessageTime(value: string, locale: string, now = new Date()): string {
  const date = new Date(value)
  if (!Number.isFinite(date.getTime())) return ''

  const sameDay =
    date.getFullYear() === now.getFullYear() &&
    date.getMonth() === now.getMonth() &&
    date.getDate() === now.getDate()
  const differentYear = date.getFullYear() !== now.getFullYear()

  return new Intl.DateTimeFormat(locale === 'zh-CN' ? 'zh-CN' : 'en-US', {
    ...(sameDay
      ? {}
      : {
          ...(differentYear ? { year: 'numeric' as const } : {}),
          month: 'short' as const,
          day: 'numeric' as const,
        }),
    hour: 'numeric',
    minute: '2-digit',
  }).format(date)
}
