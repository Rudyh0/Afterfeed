<script lang="ts">
  import { post } from '../../lib/api'
  import { errorMessage } from '../../lib/errors'
  import { locale, t } from '../../lib/i18n/index.svelte'
  import { navigate } from '../../lib/router.svelte'
  import { loadSession } from '../../lib/session.svelte'
  import Step from './Step.svelte'

  let username = $state('')
  let password = $state('')
  let confirm = $state('')
  let error = $state('')
  let busy = $state(false)

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    if (password.length < 10) {
      error = t('auth.password_too_short')
      return
    }
    if (password !== confirm) {
      error = t('auth.password_mismatch')
      return
    }
    busy = true
    try {
      await post('/setup', { username, password, locale: locale() })
      await loadSession()
      navigate('/take-in/sources', { replace: true })
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }
</script>

<form onsubmit={submit}>
  <Step title={t('takein.account_title')} body={t('takein.account_body')}>
    <div class="field">
      <label for="username">{t('auth.username')}</label>
      <input id="username" class="input" bind:value={username} autocomplete="username" autocapitalize="none" spellcheck="false" required />
    </div>
    <div class="field">
      <label for="password">{t('auth.password')}</label>
      <input id="password" class="input" type="password" bind:value={password} autocomplete="new-password" aria-describedby="rules" required />
      <p id="rules" class="hint">{t('auth.password_rules')}</p>
    </div>
    <div class="field">
      <label for="confirm">{t('auth.confirm_password')}</label>
      <input id="confirm" class="input" type="password" bind:value={confirm} autocomplete="new-password" required />
    </div>
    {#if error}<p class="error-text" role="alert">{error}</p>{/if}

    {#snippet footer()}
      <a class="btn btn-lg" href="/take-in">{t('common.back')}</a>
      <button class="btn btn-primary btn-lg" type="submit" disabled={busy}>{t('common.continue')}</button>
    {/snippet}
  </Step>
</form>

<style>
  form {
    width: 100%;
    display: flex;
    justify-content: center;
  }

  .hint {
    color: var(--text-muted);
    font-size: 13px;
  }
</style>
