<script lang="ts">
  // Shows a message instead of a frozen screen when a page fails to render.
  import type { Snippet } from 'svelte'
  import { t } from '../lib/i18n/index.svelte'
  import { hideSplash } from '../lib/splash'

  let { children }: { children: Snippet } = $props()

  function onerror(error: unknown) {
    console.error(error)
    hideSplash()
  }
</script>

<svelte:boundary {onerror}>
  {@render children()}
  {#snippet failed(_error, reset)}
    <div class="failed" role="alert">
      <p>{t('error.page_failed')}</p>
      <div class="actions">
        <button class="btn" type="button" onclick={reset}>{t('common.retry')}</button>
        <button class="btn btn-primary" type="button" onclick={() => location.reload()}>{t('error.reload')}</button>
      </div>
    </div>
  {/snippet}
</svelte:boundary>

<style>
  .failed {
    max-width: 480px;
    margin: 80px auto;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
    padding: 0 16px;
    color: var(--text-2);
    font-size: 15px;
    text-align: center;
  }

  .actions {
    display: flex;
    gap: 8px;
  }
</style>
