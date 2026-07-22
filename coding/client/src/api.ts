const apiOrigin = (import.meta.env.VITE_API_ORIGIN ?? '').trim().replace(/\/$/, '')

// apiURL uses the Vite proxy by default. Set VITE_API_ORIGIN to the Go service
// origin only when the client and API are deployed on different origins.
export function apiURL(path: string): string {
  const normalized = path.startsWith('/') ? path : `/${path}`
  return `${apiOrigin}/api${normalized}`
}

export class APIError extends Error {
  readonly code?: string

  constructor(message: string, code?: string) {
    super(message)
    this.name = 'APIError'
    this.code = code
  }
}

export function isAPIError(error: unknown, code: string): error is APIError {
  return error instanceof APIError && error.code === code
}
