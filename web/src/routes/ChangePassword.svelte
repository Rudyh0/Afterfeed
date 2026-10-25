<script lang="ts">
  import { onMount } from 'svelte'
  import CenterCard from '../components/CenterCard.svelte'
  import { post } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { t } from '../lib/i18n/index.svelte'
  import { loadSession } from '../lib/session.svelte'
  import { hideSplash } from '../lib/splash'

  let next = $state('')
  let confirm = $state('')
  let error = $state('')
  let busy = $state(false)

  onMount(hideSplash)

  async function submit(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    if (next.length < 10) {
      error = t('auth.password_too_short')
      return
    }
    if (next !== confirm) {
      error = t('auth.password_mismatch')
      return
    }
    busy = true
    try {
      await post('/account/password', { current: '', new: next })
      await loadSession()
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }
</script>

<CenterCard>
  <div class="head">
    <h1>{t('auth.change_password_title')}</h1>
    <p>{t('auth.change_password_body')}</p>
  </div>
  <form onsubmit={submit}>
    <div class="field">
      <label for="new">{t('auth.new_password')}</label>
      <input id="new" class="input" type="password" bind:value={next} autocomplete="new-password" aria-describedby="rules" required />
      <p id="rules" class="hint">{t('auth.password_rules')}</p>
    </div>
    <div class="field">
      <label for="confirm">{t('auth.confirm_password')}</label>
      <input id="confirm" class="input" type="password" bind:value={confirm} autocomplete="new-password" required />
    </div>
    {#if error}<p class="error-text" role="alert">{error}</p>{/if}
    <button class="btn btn-primary btn-lg" type="submit" disabled={busy}>{t('common.save')}</button>
  </form>
</CenterCard>

<style>
  .head {
    display: flex;
    flex-direction: column;
    gap: 6px;
    text-align: center;
  }

  h1 {
    font-size: 22px;
    font-weight: 600;
    letter-spacing: -0.02em;
  }

  .head p,
  .hint {
    color: var(--text-3);
    font-size: 14px;
  }

  .hint {
    color: var(--text-muted);
    font-size: 13px;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .btn-lg {
    width: 100%;
  }
</style>
