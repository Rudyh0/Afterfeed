<script lang="ts">
  import QRCode from 'qrcode'
  import { get, post } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { t } from '../lib/i18n/index.svelte'
  import { session } from '../lib/session.svelte'
  import { ApiError } from '../lib/api'
  import Icon from './Icon.svelte'

  interface Security {
    totp_enabled: boolean
    recovery_codes_left: number
    reset_2fa_command: string
  }

  let security = $state<Security | null>(null)
  let stage = $state<'idle' | 'setup' | 'codes' | 'disable' | 'regenerate'>('idle')
  let secret = $state('')
  let qr = $state('')
  let code = $state('')
  let password = $state('')
  let codes = $state<string[]>([])
  let error = $state('')
  let busy = $state(false)
  let copied = $state(false)

  async function load() {
    security = await get<Security>('/account/security')
  }
  load()

  async function begin() {
    error = ''
    busy = true
    try {
      const res = await post<{ secret: string; uri: string }>('/account/2fa/begin')
      secret = res.secret
      qr = await QRCode.toString(res.uri, { type: 'svg', margin: 1, errorCorrectionLevel: 'M' })
      code = ''
      stage = 'setup'
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }

  async function enable(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    busy = true
    try {
      const res = await post<{ recovery_codes: string[] }>('/account/2fa/enable', { code })
      codes = res.recovery_codes
      stage = 'codes'
      if (session.user) session.user.totp_enabled = true
    } catch (err) {
      error = err instanceof ApiError && err.code === 'auth.invalid_code' ? t('settings.code_invalid') : errorMessage(err)
    } finally {
      busy = false
    }
  }

  async function disable(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    busy = true
    try {
      await post('/account/2fa/disable', { password })
      password = ''
      stage = 'idle'
      if (session.user) session.user.totp_enabled = false
      await load()
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }

  async function regenerate(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    busy = true
    try {
      const res = await post<{ recovery_codes: string[] }>('/account/2fa/recovery-codes', { password })
      password = ''
      codes = res.recovery_codes
      stage = 'codes'
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }

  function downloadCodes() {
    const text = `Afterfeed recovery codes\n\n${codes.join('\n')}\n`
    const url = URL.createObjectURL(new Blob([text], { type: 'text/plain' }))
    const a = document.createElement('a')
    a.href = url
    a.download = 'afterfeed-recovery-codes.txt'
    a.click()
    URL.revokeObjectURL(url)
  }

  async function finishCodes() {
    codes = []
    stage = 'idle'
    await load()
  }

  async function copyCommand() {
    if (!security) return
    try {
      await navigator.clipboard.writeText(security.reset_2fa_command)
      copied = true
      setTimeout(() => (copied = false), 2000)
    } catch {
      // Clipboard not available over plain HTTP.
    }
  }
</script>

<div class="row">
  <div class="text">
    <p class="label">{t('settings.two_factor')}</p>
    <p class="sub">{t('settings.two_factor_body')}</p>
  </div>
  {#if security}
    <span class="pill" class:pill-accent={security.totp_enabled}>
      {security.totp_enabled ? t('settings.on') : t('settings.off')}
    </span>
  {/if}
</div>

{#if security && stage === 'idle'}
  {#if security.totp_enabled}
    <p class="sub">{t('settings.recovery_left', { count: security.recovery_codes_left })}</p>
    <div class="actions">
      <button class="btn" type="button" onclick={() => ((stage = 'regenerate'), (error = ''))}>{t('settings.new_codes')}</button>
      <button class="btn btn-danger" type="button" onclick={() => ((stage = 'disable'), (error = ''))}>{t('settings.disable')}</button>
    </div>
  {:else}
    <div class="actions">
      <button class="btn btn-primary" type="button" onclick={begin} disabled={busy}>{t('settings.enable')}</button>
    </div>
  {/if}
{/if}

{#if stage === 'setup'}
  <div class="setup">
    <div class="qr" aria-hidden="true">{@html qr}</div>
    <div class="setup-text">
      <p>{t('settings.scan')}</p>
      <p class="sub">{t('settings.manual_key')}</p>
      <code class="secret">{secret.match(/.{1,4}/g)?.join(' ')}</code>
      <form onsubmit={enable}>
        <div class="field">
          <label for="totp-code">{t('settings.confirm_code')}</label>
          <input id="totp-code" class="input code" bind:value={code} inputmode="numeric" autocomplete="one-time-code" maxlength="6" required />
        </div>
        {#if error}<p class="error-text" role="alert">{error}</p>{/if}
        <div class="actions">
          <button class="btn" type="button" onclick={() => (stage = 'idle')}>{t('common.cancel')}</button>
          <button class="btn btn-primary" type="submit" disabled={busy || code.length < 6}>{t('settings.enable')}</button>
        </div>
      </form>
    </div>
  </div>
{/if}

{#if stage === 'codes'}
  <div class="codes-box">
    <p class="label">{t('settings.recovery_title')}</p>
    <p class="sub">{t('settings.recovery_body')}</p>
    <ul class="codes">
      {#each codes as c (c)}<li><code>{c}</code></li>{/each}
    </ul>
    <div class="actions">
      <button class="btn" type="button" onclick={downloadCodes}><Icon name="download" size={18} />{t('settings.download_codes')}</button>
      <button class="btn btn-primary" type="button" onclick={finishCodes}>{t('settings.codes_saved')}</button>
    </div>
  </div>
{/if}

{#if stage === 'disable' || stage === 'regenerate'}
  <form class="confirm" onsubmit={stage === 'disable' ? disable : regenerate}>
    <p class="sub">{stage === 'disable' ? t('settings.disable_confirm') : t('settings.new_codes_confirm')}</p>
    <div class="field">
      <label for="confirm-password">{t('auth.password')}</label>
      <input id="confirm-password" class="input" type="password" bind:value={password} autocomplete="current-password" required />
    </div>
    {#if error}<p class="error-text" role="alert">{error}</p>{/if}
    <div class="actions">
      <button class="btn" type="button" onclick={() => ((stage = 'idle'), (password = ''))}>{t('common.cancel')}</button>
      <button class="btn" class:btn-primary={stage === 'regenerate'} class:btn-danger={stage === 'disable'} type="submit" disabled={busy}>
        {stage === 'disable' ? t('settings.disable') : t('settings.new_codes')}
      </button>
    </div>
  </form>
{/if}

{#if security}
  <div class="reset">
    <p class="sub">{t('settings.reset_command')}</p>
    <div class="command">
      <code>{security.reset_2fa_command}</code>
      <button class="icon-btn" type="button" aria-label={copied ? t('common.copied') : t('common.copy')} onclick={copyCommand}>
        <Icon name={copied ? 'check' : 'copy'} size={18} />
      </button>
    </div>
  </div>
{/if}

<style>
  .row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
  }

  .label {
    font-size: 15px;
    font-weight: 600;
  }

  .sub {
    color: var(--text-3);
    font-size: 14px;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .setup {
    display: flex;
    flex-wrap: wrap;
    gap: 24px;
    padding: 20px;
    border-radius: var(--radius-lg);
    background: var(--surface-2);
  }

  .qr {
    width: 180px;
    height: 180px;
    flex: 0 0 180px;
    padding: 8px;
    border-radius: var(--radius-md);
    background: #ffffff;
  }

  .qr :global(svg) {
    width: 100%;
    height: 100%;
  }

  .setup-text {
    flex: 1 1 260px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .setup-text form {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin-top: 6px;
  }

  .secret {
    font-size: 14px;
    word-break: break-all;
  }

  .code {
    max-width: 200px;
    font-size: 18px;
    letter-spacing: 0.12em;
    font-variant-numeric: tabular-nums;
  }

  .codes-box,
  .confirm {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 20px;
    border-radius: var(--radius-lg);
    background: var(--surface-2);
  }

  .codes {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 6px 24px;
    margin: 4px 0;
    padding: 0;
    list-style: none;
  }

  .codes code {
    font-size: 15px;
    font-variant-numeric: tabular-nums;
  }

  .reset {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding-top: 16px;
    border-top: 1px solid var(--divider);
  }

  .command {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 4px 4px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }

  .command code {
    flex: 1 1 auto;
    min-width: 0;
    overflow-x: auto;
    white-space: nowrap;
    font-size: 13px;
  }

  code {
    font-family: ui-monospace, 'SF Mono', Menlo, Consolas, monospace;
  }
</style>
