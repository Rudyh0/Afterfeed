<script lang="ts">
  import { onMount } from 'svelte'
  import { post } from '../../lib/api'
  import { errorMessage } from '../../lib/errors'
  import { t } from '../../lib/i18n/index.svelte'
  import { library, loadLibrary } from '../../lib/library.svelte'
  import { navigate } from '../../lib/router.svelte'
  import { session } from '../../lib/session.svelte'
  import Step from './Step.svelte'
  import { setQueue } from './takein.svelte'

  let busy = $state(false)
  let error = $state('')

  onMount(loadLibrary)

  async function finish() {
    busy = true
    error = ''
    try {
      await post('/take-in/done')
      session.takeInDone = true
      setQueue([])
      navigate('/', { replace: true })
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }
</script>

<Step title={t('takein.done_title')} body={t('takein.done_body', { action: t('nav.add_source') })}>
  {#if library.sources.length > 0}
    <ul class="card sources">
      {#each library.sources as s (s.platform)}
        <li>
          <span class="name">{t(`platform.${s.platform}`)}</span>
          <span class="muted tabular">
            {#if s.memories > 0}{t('count.memories', { count: s.memories })}{/if}
            {#if s.memories > 0 && s.first_year}· {s.first_year === s.last_year ? s.first_year : `${s.first_year}–${s.last_year}`}{/if}
          </span>
        </li>
      {/each}
    </ul>
  {/if}
  {#if error}<p class="error-text" role="alert">{error}</p>{/if}

  {#snippet footer()}
    <a class="btn btn-lg" href="/take-in/sources">{t('takein.add_another')}</a>
    <button class="btn btn-primary btn-lg" type="button" disabled={busy} onclick={finish}>{t('takein.finish')}</button>
  {/snippet}
</Step>

<style>
  .sources {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .sources li {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    padding: 14px 16px;
  }

  .sources li + li {
    border-top: 1px solid var(--divider);
  }

  .name {
    font-weight: 600;
  }

  .muted {
    color: var(--text-3);
  }
</style>
