<script lang="ts">
  import { onMount, tick } from 'svelte'
  import Dialog from '../components/Dialog.svelte'
  import Icon from '../components/Icon.svelte'
  import TwoFactor from '../components/TwoFactor.svelte'
  import { ApiError, get, patch, post } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { locale, localeNames, locales, setLocale, t, type Locale } from '../lib/i18n/index.svelte'
  import { navigate, type Params } from '../lib/router.svelte'
  import { session } from '../lib/session.svelte'
  import { hideSplash } from '../lib/splash'
  import { setTheme, theme, type ThemePreference } from '../lib/theme.svelte'

  let { params = {} }: { params?: Params } = $props()

  interface SettingsResponse {
    map: { mode: 'external' | 'local' | 'off'; style_url: string }
    vault: { autolock_minutes: number }
  }

  let settings = $state<SettingsResponse | null>(null)
  let mapMode = $state<'external' | 'local' | 'off'>('external')
  let styleUrl = $state('')
  let mapError = $state('')
  let mapSaved = $state(false)
  let autolock = $state(5)
  let pinSet = $state<boolean | null>(null)
  let pinOpen = $state(false)
  let pinCurrent = $state('')
  let pinNext = $state('')
  let pinRepeat = $state('')
  let pinError = $state('')
  let pinSaved = $state(false)
  let pinBusy = $state(false)

  let current = $state('')
  let next = $state('')
  let confirm = $state('')
  let pwError = $state('')
  let pwSaved = $state(false)
  let pwBusy = $state(false)

  onMount(async () => {
    try {
      settings = await get<SettingsResponse>('/settings')
      mapMode = settings.map.mode
      styleUrl = settings.map.style_url
      autolock = settings.vault.autolock_minutes
      pinSet = (await get<{ pin_set: boolean }>('/vault')).pin_set
    } finally {
      hideSplash()
    }
    if (params.section) {
      await tick()
      document.getElementById(params.section)?.scrollIntoView({ block: 'start' })
    }
  })

  async function changeLanguage(l: Locale) {
    setLocale(l)
    await patch('/account', { locale: l })
    if (session.user) session.user.locale = l
  }

  async function saveMap() {
    mapError = ''
    mapSaved = false
    try {
      await patch('/settings', { map: { mode: mapMode, style_url: styleUrl.trim() } })
      mapSaved = true
      setTimeout(() => (mapSaved = false), 2000)
    } catch (err) {
      mapError = err instanceof ApiError && err.code === 'settings.invalid_style_url' ? t('settings.invalid_style_url') : errorMessage(err)
    }
  }

  async function changePin(e: SubmitEvent) {
    e.preventDefault()
    pinError = ''
    if (!/^\d{6}$/.test(pinNext)) {
      pinError = t('vault.pin_digits')
      return
    }
    if (pinNext !== pinRepeat) {
      pinError = t('vault.mismatch')
      return
    }
    pinBusy = true
    try {
      await post('/vault/pin/change', { current: pinCurrent, pin: pinNext })
      pinOpen = false
      pinCurrent = pinNext = pinRepeat = ''
      pinSaved = true
      setTimeout(() => (pinSaved = false), 2000)
    } catch (err) {
      pinError = errorMessage(err)
    } finally {
      pinBusy = false
    }
  }

  async function saveAutolock() {
    await patch('/settings', { vault: { autolock_minutes: autolock } })
  }

  async function changePassword(e: SubmitEvent) {
    e.preventDefault()
    pwError = ''
    pwSaved = false
    if (next.length < 10) {
      pwError = t('auth.password_too_short')
      return
    }
    if (next !== confirm) {
      pwError = t('auth.password_mismatch')
      return
    }
    pwBusy = true
    try {
      await post('/account/password', { current, new: next })
      current = next = confirm = ''
      pwSaved = true
    } catch (err) {
      pwError = errorMessage(err)
    } finally {
      pwBusy = false
    }
  }

  async function signOut() {
    await post('/auth/logout')
    session.authenticated = false
    session.user = null
    navigate('/', { replace: true })
  }

  const themes: { value: ThemePreference; label: string; icon: string }[] = [
    { value: 'system', label: 'settings.theme_system', icon: 'settings' },
    { value: 'light', label: 'settings.theme_light', icon: 'sun' },
    { value: 'dark', label: 'settings.theme_dark', icon: 'moon' },
  ]
  const autolockOptions = [1, 2, 5, 10, 15, 30]
</script>

<div class="settings">
  <h1 class="title">{t('settings.title')}</h1>

  <section id="account" class="card section" aria-labelledby="h-account">
    <h2 id="h-account">{t('settings.account')}</h2>
    {#if session.user}
      <p class="sub">{t('settings.signed_in_as', { name: session.user.username })}</p>
    {/if}
    <form class="form" onsubmit={changePassword}>
      <p class="label">{t('settings.change_password')}</p>
      <div class="field">
        <label for="pw-current">{t('auth.current_password')}</label>
        <input id="pw-current" class="input" type="password" bind:value={current} autocomplete="current-password" required />
      </div>
      <div class="two">
        <div class="field">
          <label for="pw-new">{t('auth.new_password')}</label>
          <input id="pw-new" class="input" type="password" bind:value={next} autocomplete="new-password" required />
        </div>
        <div class="field">
          <label for="pw-confirm">{t('auth.confirm_password')}</label>
          <input id="pw-confirm" class="input" type="password" bind:value={confirm} autocomplete="new-password" required />
        </div>
      </div>
      <p class="hint">{t('auth.password_rules')}</p>
      {#if pwError}<p class="error-text" role="alert">{pwError}</p>{/if}
      {#if pwSaved}<p class="ok" role="status">{t('settings.password_changed')}</p>{/if}
      <div class="actions">
        <button class="btn" type="submit" disabled={pwBusy}>{t('settings.change_password')}</button>
        <button class="btn" type="button" onclick={signOut}><Icon name="logout" size={18} />{t('common.sign_out')}</button>
      </div>
    </form>
  </section>

  <section id="security" class="card section" aria-labelledby="h-security">
    <h2 id="h-security">{t('settings.security')}</h2>
    <TwoFactor />
  </section>

  <section id="appearance" class="card section" aria-labelledby="h-appearance">
    <h2 id="h-appearance">{t('settings.appearance')}</h2>
    <div class="field">
      <span class="field-label" id="theme-label">{t('settings.theme')}</span>
      <div class="segmented" role="group" aria-labelledby="theme-label">
        {#each themes as th (th.value)}
          <button type="button" aria-pressed={theme.preference === th.value} onclick={() => setTheme(th.value)}>
            {t(th.label)}
          </button>
        {/each}
      </div>
    </div>
    <div class="field">
      <label for="language">{t('settings.language')}</label>
      <select id="language" class="input select" value={locale()} onchange={(e) => changeLanguage((e.currentTarget as HTMLSelectElement).value as Locale)}>
        {#each locales as l (l)}
          <option value={l}>{localeNames[l]}</option>
        {/each}
      </select>
    </div>
  </section>

  <section id="map" class="card section" aria-labelledby="h-map">
    <h2 id="h-map">{t('settings.map')}</h2>
    {#if settings}
      <div class="field">
        <span class="field-label" id="map-label">{t('settings.map_source')}</span>
        <div class="segmented wrap" role="group" aria-labelledby="map-label">
          <button type="button" aria-pressed={mapMode === 'external'} onclick={() => (mapMode = 'external')}>{t('settings.map_external')}</button>
          <button type="button" aria-pressed={mapMode === 'local'} onclick={() => (mapMode = 'local')}>{t('settings.map_local')}</button>
          <button type="button" aria-pressed={mapMode === 'off'} onclick={() => (mapMode = 'off')}>{t('settings.map_off')}</button>
        </div>
      </div>
      {#if mapMode !== 'off'}
        <div class="field">
          <label for="style-url">{t('settings.map_style_url')}</label>
          <input
            id="style-url"
            class="input"
            bind:value={styleUrl}
            placeholder={mapMode === 'external' ? 'https://tiles.openfreemap.org/styles/positron' : 'https://'}
            inputmode="url"
            spellcheck="false"
          />
          <p class="hint">{mapMode === 'external' ? `${t('settings.map_style_hint')} ${t('settings.map_default')}.` : t('settings.map_local_hint')}</p>
        </div>
      {/if}
      {#if mapMode === 'external'}<p class="sub">{t('settings.map_privacy')}</p>{/if}
      {#if mapError}<p class="error-text" role="alert">{mapError}</p>{/if}
      <div class="actions">
        <button class="btn" type="button" onclick={saveMap}>{t('common.save')}</button>
        {#if mapSaved}<span class="ok" role="status">{t('settings.saved')}</span>{/if}
      </div>
    {/if}
  </section>

  <section id="vault" class="card section" aria-labelledby="h-vault">
    <h2 id="h-vault">{t('settings.vault')}</h2>
    <div class="field">
      <label for="autolock">{t('settings.vault_autolock')}</label>
      <select id="autolock" class="input select" bind:value={autolock} onchange={saveAutolock}>
        {#each autolockOptions as m (m)}
          <option value={m}>{t('settings.minutes', { count: m })}</option>
        {/each}
      </select>
    </div>
    {#if pinSet === true}
      <div class="actions">
        <button class="btn" type="button" onclick={() => { pinOpen = true; pinError = '' }}><Icon name="key" size={18} />{t('settings.change_pin')}</button>
        {#if pinSaved}<span class="ok" role="status">{t('vault.pin_changed')}</span>{/if}
      </div>
    {:else if pinSet === false}
      <p class="sub">{t('vault.no_pin_yet')}</p>
    {/if}
  </section>

  <Dialog open={pinOpen} title={t('vault.change_title')} onclose={() => (pinOpen = false)} width={420}>
    <form id="pin-form" class="pin-form" onsubmit={changePin}>
      <div class="field">
        <label for="pin-current">{t('vault.current_pin')}</label>
        <input id="pin-current" class="input" type="password" inputmode="numeric" maxlength="6" autocomplete="off" bind:value={pinCurrent} />
      </div>
      <div class="field">
        <label for="pin-next">{t('vault.new_pin')}</label>
        <input id="pin-next" class="input" type="password" inputmode="numeric" maxlength="6" autocomplete="off" bind:value={pinNext} />
      </div>
      <div class="field">
        <label for="pin-repeat">{t('vault.repeat_pin')}</label>
        <input id="pin-repeat" class="input" type="password" inputmode="numeric" maxlength="6" autocomplete="off" bind:value={pinRepeat} />
      </div>
      {#if pinError}<p class="error-text" role="alert">{pinError}</p>{/if}
    </form>
    {#snippet footer()}
      <button class="btn" type="button" onclick={() => (pinOpen = false)}>{t('common.cancel')}</button>
      <button class="btn btn-primary" type="submit" form="pin-form" disabled={pinBusy || pinCurrent.length !== 6 || !pinNext}>{t('common.save')}</button>
    {/snippet}
  </Dialog>

  <section id="about" class="card section" aria-labelledby="h-about">
    <h2 id="h-about">{t('settings.about')}</h2>
    <div class="about-row">
      <span class="sub">{t('settings.version')}</span>
      <span class="tabular">{session.version}</span>
    </div>
    <a class="link" href="https://rudyh0.github.io/Afterfeed/" target="_blank" rel="noreferrer">{t('settings.docs_link')}</a>
  </section>
</div>

<style>
  .settings {
    max-width: 760px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .title {
    margin-bottom: 8px;
    font-size: 32px;
    font-weight: 600;
    letter-spacing: -0.025em;
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding: 24px;
    scroll-margin-top: 24px;
  }

  h2 {
    font-size: 18px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  .label {
    font-size: 15px;
    font-weight: 600;
  }

  .sub {
    color: var(--text-3);
    font-size: 14px;
  }

  .hint {
    color: var(--text-muted);
    font-size: 13px;
  }

  .ok {
    color: var(--success);
    font-size: 14px;
    font-weight: 500;
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .two {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px;
  }

  .select {
    max-width: 280px;
  }

  .pin-form {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .segmented.wrap {
    flex-wrap: wrap;
  }

  .about-row {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }

  @media (max-width: 600px) {
    .section {
      padding: 20px 16px;
    }

    .two {
      grid-template-columns: 1fr;
    }
  }
</style>
