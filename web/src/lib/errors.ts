import { ApiError } from './api'
import { t } from './i18n/index.svelte'

// Maps API error codes to translated messages.
const messages: Record<string, string> = {
  'auth.invalid_credentials': 'auth.invalid_credentials',
  'auth.invalid_code': 'auth.invalid_code',
  'auth.password_too_short': 'auth.password_too_short',
  not_found: 'error.not_found',
}

export function errorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.code === 'auth.too_many_attempts') {
      return t('auth.too_many', { seconds: err.retryAfter ?? 30 })
    }
    const key = messages[err.code]
    if (key) return t(key)
    return t('error.internal')
  }
  if (err instanceof TypeError) return t('error.network')
  return t('error.internal')
}
