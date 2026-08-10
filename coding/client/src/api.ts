export function apiURL(path: string): string {
  const normalized = path.startsWith('/') ? path : `/${path}`
  return `/api${normalized}`
}

export function sessionURL(id: string, path: string): string {
  return apiURL(`/sessions/${encodeURIComponent(id)}${path}`)
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
