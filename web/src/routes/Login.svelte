<script lang="ts">
  import { onMount, tick } from 'svelte'
  import CenterCard from '../components/CenterCard.svelte'
  import { post } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { t } from '../lib/i18n/index.svelte'
  import { loadLibrary } from '../lib/library.svelte'
  import { loadSession, session } from '../lib/session.svelte'
  import { hideSplash } from '../lib/splash'

  let username = $state('')
  let password = $state('')
  let code = $state('')
  let error = $state('')
  let busy = $state(false)
  let codeInput = $state<HTMLInputElement>()

  onMount(hideSplash)

  async function signIn(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    busy = true
    try {
      const res = await post<{ mfa_required: boolean }>('/auth/login', { username, password })
      password = ''
      if (res.mfa_required) {
        session.mfaRequired = true
        await tick()
        codeInput?.focus()
      } else {
        await finish()
      }
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }

  async function verify(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    busy = true
    try {
      await post('/auth/mfa', { code })
      await finish()
    } catch (err) {
      error = errorMessage(err)
      code = ''
    } finally {
      busy = false
    }
  }

  async function finish() {
    await loadSession()
    loadLibrary()
  }
</script>

<CenterCard>
  <h1>{t('auth.sign_in')}</h1>

  {#if session.mfaRequired}
    <form onsubmit={verify}>
      <div class="field">
        <label for="code">{t('auth.code')}</label>
        <input
          bind:this={codeInput}
          id="code"
          class="input code"
          bind:value={code}
          autocomplete="one-time-code"
          inputmode="text"
          spellcheck="false"
          aria-describedby="code-hint"
          required
        />
        <p id="code-hint" class="hint">{t('auth.code_hint')}</p>
      </div>
      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
      <button class="btn btn-primary btn-lg" type="submit" disabled={busy || code.trim() === ''}>{t('auth.verify')}</button>
    </form>
  {:else}
    <form onsubmit={signIn}>
      <div class="field">
        <label for="username">{t('auth.username')}</label>
        <input id="username" class="input" bind:value={username} autocomplete="username" autocapitalize="none" spellcheck="false" required />
      </div>
      <div class="field">
        <label for="password">{t('auth.password')}</label>
        <input id="password" class="input" type="password" bind:value={password} autocomplete="current-password" required />
      </div>
      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
      <button class="btn btn-primary btn-lg" type="submit" disabled={busy}>{t('auth.sign_in')}</button>
    </form>
  {/if}
</CenterCard>

<style>
  h1 {
    font-size: 24px;
    font-weight: 600;
    letter-spacing: -0.02em;
    text-align: center;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .code {
    font-size: 20px;
    letter-spacing: 0.12em;
    text-align: center;
    font-variant-numeric: tabular-nums;
  }

  .hint {
    color: var(--text-muted);
    font-size: 13px;
  }

  .btn-lg {
    width: 100%;
    margin-top: 4px;
  }
</style>
