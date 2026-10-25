<script lang="ts">
  import Logo from '../components/Logo.svelte'
  import { t } from '../lib/i18n/index.svelte'
  import { loadSession } from '../lib/session.svelte'

  let retrying = $state(false)

  async function retry() {
    retrying = true
    await loadSession()
    retrying = false
  }
</script>

<div class="unreachable">
  <Logo width={96} />
  <p>{t('app.unreachable')}</p>
  <button class="btn" type="button" onclick={retry} disabled={retrying}>{t('app.retry')}</button>
</div>

<style>
  .unreachable {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 20px;
    min-height: 100vh;
    padding: 24px;
    color: var(--text);
    font-size: 16px;
    text-align: center;
  }
</style>
