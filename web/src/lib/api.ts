// A small client for the Afterfeed API. Every request carries the X-Afterfeed
// header, which the server requires for anything that changes data.

export class ApiError extends Error {
  status: number
  code: string
  retryAfter?: number

  constructor(status: number, code: string, message: string, retryAfter?: number) {
    super(message)
    this.status = status
    this.code = code
    this.retryAfter = retryAfter
  }
}

type Listener = () => void
const unauthorizedListeners = new Set<Listener>()

/** Registers a callback for when the server says the session is no longer valid. */
export function onUnauthorized(fn: Listener): () => void {
  unauthorizedListeners.add(fn)
  return () => unauthorizedListeners.delete(fn)
}

export async function api<T = unknown>(
  method: string,
  path: string,
  body?: unknown,
  init: RequestInit = {},
): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('X-Afterfeed', '1')
  let payload: BodyInit | undefined
  if (body instanceof Blob || body instanceof ArrayBuffer) {
    payload = body
  } else if (body !== undefined) {
    headers.set('Content-Type', 'application/json')
    payload = JSON.stringify(body)
  }

  const res = await fetch(`/api${path}`, {
    ...init,
    method,
    headers,
    body: payload,
    credentials: 'same-origin',
  })

  if (!res.ok) {
    let code = 'internal'
    let message = res.statusText
    let retryAfter: number | undefined
    try {
      const data = await res.json()
      code = data?.error?.code ?? code
      message = data?.error?.message ?? message
      retryAfter = data?.error?.retry_after
    } catch {
      // Not a JSON error body.
    }
    if (res.status === 401 && code === 'auth.required') {
      unauthorizedListeners.forEach((fn) => fn())
    }
    throw new ApiError(res.status, code, message, retryAfter)
  }

  if (res.status === 204) return undefined as T
  const type = res.headers.get('Content-Type') ?? ''
  if (!type.includes('application/json')) return undefined as T
  return (await res.json()) as T
}

export const get = <T>(path: string, init?: RequestInit) => api<T>('GET', path, undefined, init)
export const post = <T>(path: string, body?: unknown) => api<T>('POST', path, body ?? {})
export const patch = <T>(path: string, body?: unknown) => api<T>('PATCH', path, body ?? {})
export const put = <T>(path: string, body?: unknown) => api<T>('PUT', path, body ?? {})
export const del = <T>(path: string) => api<T>('DELETE', path)

/** URL of a media file variant: thumb, preview, original or download. */
export function mediaUrl(id: string, variant: 'thumb' | 'preview' | 'original' | 'download' = 'thumb'): string {
  return `/api/media/${encodeURIComponent(id)}/${variant}`
}
