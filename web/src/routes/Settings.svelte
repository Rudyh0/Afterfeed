<script lang="ts">
  import { onMount, tick } from 'svelte'
  import Dialog from '../components/Dialog.svelte'
  import Icon from '../components/Icon.svelte'
  import TwoFactor from '../components/TwoFactor.svelte'
  import { ApiError, get, patch, post } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { formatBytes, formatShortDate, locale, localeNames, locales, setLocale, t, type Locale } from '../lib/i18n/index.svelte'
  import { loadLibrary } from '../lib/library.svelte'
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
  interface ImportSummary {
    id: number
    platform: string
    status: 'queued' | 'running' | 'needs_parts' | 'done' | 'failed' | 'cancelled'
    phase: string
    created_at: string
    finished_at?: string
    report?: { new: number; new_conversations: number }
  }

  let imports = $state<ImportSummary[] | null>(null)

  interface SourceInfo {
    platform: string
    first_year?: number
    last_year?: number
    memories: number
    conversations: number
    uploads: number
    uploads_size: number
    removable: boolean
  }
  let sources = $state<SourceInfo[] | null>(null)
  let removing = $state<SourceInfo | null>(null)
  let removeOpen = $state(false)
  let removePassword = $state('')
  let removeUploads = $state(false)
  let removeError = $state('')
  let removeBusy = $state(false)
  let removedMessage = $state('')
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
      imports = (await get<{ imports: ImportSummary[] }>('/imports')).imports
      sources = (await get<{ sources: SourceInfo[] }>('/sources')).sources
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

  function importLine(i: ImportSummary): string {
    switch (i.status) {
      case 'done': {
        const parts = [t('settings.import_new', { count: i.report?.new ?? 0 })]
        if (i.report?.new_conversations) parts.push(t('count.conversations', { count: i.report.new_conversations }))
        return parts.join(' · ')
      }
      case 'needs_parts':
        return t('settings.import_needs_parts')
      case 'failed':
        return t('takein.report_failed')
      case 'cancelled':
        return t('settings.import_cancelled')
      default:
        return t(`takein.phase.${i.phase || 'queued'}`)
    }
  }

  function platformName(p: string): string {
    return p === 'afterfeed' ? t('nav.own_memories') : t(`platform.${p}`)
  }

  // "596 memories and 318 conversations"
  function sourceItems(s: SourceInfo): string {
    const parts: string[] = []
    if (s.memories) parts.push(t('count.memories', { count: s.memories }))
    if (s.conversations) parts.push(t('count.conversations', { count: s.conversations }))
    return parts.join(` ${t('common.and')} `)
  }

  function openRemove(s: SourceInfo) {
    removing = s
    removePassword = ''
    removeUploads = false
    removeError = ''
    removedMessage = ''
    removeOpen = true
  }

  async function removeSource(e: SubmitEvent) {
    e.preventDefault()
    if (!removing) return
    removeBusy = true
    removeError = ''
    const name = platformName(removing.platform)
    try {
      await post(`/sources/${removing.platform}/remove`, { password: removePassword, delete_uploads: removeUploads })
      removeOpen = false
      removePassword = ''
      removedMessage = t('settings.source_removed', { platform: name })
      sources = (await get<{ sources: SourceInfo[] }>('/sources')).sources
      imports = (await get<{ imports: ImportSummary[] }>('/imports')).imports
      loadLibrary()
    } catch (err) {
      removeError =
        err instanceof ApiError && err.code === 'auth.invalid_credentials' ? t('auth.wrong_password') : errorMessage(err)
    } finally {
      removeBusy = false
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

  <section id="sources" class="card section" aria-labelledby="h-sources">
    <h2 id="h-sources">{t('settings.sources')}</h2>
    {#if sources && sources.length > 0}
      <p class="label">{t('settings.in_archive')}</p>
      <ul class="imports">
        {#each sources as src (src.platform)}
          <li class="source">
            <span class="import-text">
              <span class="import-title">{platformName(src.platform)}{#if src.first_year}<span class="sub tabular">{src.first_year === src.last_year ? src.first_year : `${src.first_year}–${src.last_year}`}</span>{/if}</span>
              <span class="sub">{sourceItems(src)}</span>
            </span>
            {#if src.removable}
              <button class="btn btn-danger" type="button" aria-label={t('settings.remove_source', { platform: platformName(src.platform) })}
                      onclick={() => openRemove(src)}><Icon name="trash" size={18} />{t('common.remove')}</button>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
    {#if removedMessage}<p class="ok" role="status">{removedMessage}</p>{/if}
    {#if imports && imports.length > 0}
      <p class="label">{t('settings.import_history')}</p>
      <ul class="imports">
        {#each imports as i (i.id)}
          <li>
            <a href={`/take-in/import/${i.id}`}>
              <span class="import-text">
                <span class="import-title">{i.platform ? t(`platform.${i.platform}`) : t('common.unknown')} <span class="sub tabular">{formatShortDate(i.finished_at || i.created_at)}</span></span>
                <span class="sub" class:attention={i.status === 'needs_parts' || i.status === 'failed'}>{importLine(i)}</span>
              </span>
              <Icon name="chevron-right" size={18} />
            </a>
          </li>
        {/each}
      </ul>
    {:else if imports}
      <p class="sub">{t('settings.no_imports')}</p>
    {/if}
    <div class="actions">
      <a class="btn" href="/take-in/sources"><Icon name="plus" size={18} />{t('nav.add_source')}</a>
    </div>
  </section>

  <Dialog open={removeOpen} title={removing ? t('settings.remove_title', { platform: platformName(removing.platform) }) : ''}
          onclose={() => { if (!removeBusy) removeOpen = false }} width={480}>
    {#if removing}
      <form id="remove-form" class="pin-form" onsubmit={removeSource}>
        <p>{t('settings.remove_body', { items: sourceItems(removing) })}</p>
        <p class="sub">{t('settings.remove_again')}</p>
        {#if removing.uploads > 0}
          <div class="check-field">
            <label class="check"><input type="checkbox" bind:checked={removeUploads} />{t('settings.remove_uploads', { size: formatBytes(removing.uploads_size) })}</label>
            <p class="hint">{t('settings.remove_uploads_hint')}</p>
          </div>
        {/if}
        <div class="field">
          <label for="remove-password">{t('settings.your_password')}</label>
          <input id="remove-password" class="input" type="password" autocomplete="current-password" bind:value={removePassword} />
        </div>
        {#if removeError}<p class="error-text" role="alert">{removeError}</p>{/if}
      </form>
    {/if}
    {#snippet footer()}
      <button class="btn" type="button" disabled={removeBusy} onclick={() => (removeOpen = false)}>{t('common.cancel')}</button>
      <button class="btn btn-primary danger-fill" type="submit" form="remove-form" disabled={removeBusy || !removePassword}>
        {removeBusy ? t('settings.removing') : removing ? t('settings.remove_source', { platform: platformName(removing.platform) }) : ''}
      </button>
    {/snippet}
  </Dialog>

  <section id="about" class="card section" aria-labelledby="h-about">
    <h2 id="h-about">{t('settings.about')}</h2>
    <div class="about-row">
      <span class="sub">{t('settings.version')}</span>
      <span class="tabular">{session.version}</span>
    </div>
    <div class="about-row">
      <span class="sub">{t('settings.place_names')}</span>
      <a class="link inline" href="https://www.geonames.org" target="_blank" rel="noreferrer">GeoNames (CC BY 4.0)</a>
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
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .link.inline {
    min-height: 0;
  }

  .imports {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .imports li + li {
    border-top: 1px solid var(--divider);
  }

  .imports a {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 0;
    color: var(--text);
    text-decoration: none;
  }

  .imports a:hover .import-title {
    text-decoration: underline;
  }

  .imports a :global(svg) {
    color: var(--text-muted);
  }

  .import-text {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .import-title {
    font-size: 15px;
    font-weight: 600;
  }

  .import-title .sub {
    margin-left: 6px;
    font-weight: 400;
  }

  .attention {
    color: var(--danger);
  }

  .source {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px 0;
  }

  .check-field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .check {
    display: flex;
    align-items: flex-start;
    gap: 10px;
    font-size: 14px;
    line-height: 1.4;
  }

  .check input {
    flex: 0 0 auto;
    width: 18px;
    height: 18px;
    margin-top: 1px;
    accent-color: var(--ink);
  }

  .check-field .hint {
    padding-left: 28px;
  }

  .danger-fill {
    border-color: var(--danger);
    background: var(--danger);
    color: #fff;
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
