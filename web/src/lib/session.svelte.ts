// What the app knows about the current visitor: whether Afterfeed still needs
// to be set up, whether they are signed in, and their account.

import { get, onUnauthorized } from './api'
import { isLocale, setLocale } from './i18n/index.svelte'

export interface User {
  username: string
  locale: string
  totp_enabled: boolean
  must_change_password: boolean
  vault_pin_set: boolean
}

export interface SessionState {
  loaded: boolean
  unreachable: boolean
  version: string
  setupRequired: boolean
  authenticated: boolean
  mfaRequired: boolean
  takeInDone: boolean
  user: User | null
}

export const session = $state<SessionState>({
  loaded: false,
  unreachable: false,
  version: '',
  setupRequired: false,
  authenticated: false,
  mfaRequired: false,
  takeInDone: false,
  user: null,
})

interface StateResponse {
  version: string
  setup_required: boolean
  authenticated: boolean
  mfa_required?: boolean
  take_in_done?: boolean
  user?: User
}

export async function loadSession() {
  try {
    const s = await get<StateResponse>('/state')
    session.version = s.version
    session.setupRequired = s.setup_required
    session.authenticated = s.authenticated
    session.mfaRequired = !!s.mfa_required
    session.takeInDone = !!s.take_in_done
    session.user = s.user ?? null
    session.unreachable = false
    if (s.user && isLocale(s.user.locale)) setLocale(s.user.locale)
  } catch {
    session.unreachable = true
  } finally {
    session.loaded = true
  }
}

onUnauthorized(() => {
  session.authenticated = false
  session.user = null
})
