const apiOrigin = (import.meta.env.VITE_API_ORIGIN ?? '').trim().replace(/\/$/, '')

// apiURL uses the Vite proxy by default. Set VITE_API_ORIGIN to the Go service
// origin only when the front-end and API are deployed on different origins.
export function apiURL(path: string): string {
  const normalized = path.startsWith('/') ? path : `/${path}`
  return `${apiOrigin}/api${normalized}`
}
