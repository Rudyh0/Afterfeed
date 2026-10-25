<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { post } from '../../lib/api'
  import { t } from '../../lib/i18n/index.svelte'
  import { navigate } from '../../lib/router.svelte'
  import { session } from '../../lib/session.svelte'
  import Step from './Step.svelte'
  import { platforms, setQueue, takeIn, type Platform } from './takein.svelte'

  let selected = $state<Platform[]>(takeIn.queue.length ? [...takeIn.queue] : [])
  let error = $state('')

  function toggle(p: Platform) {
    selected = selected.includes(p) ? selected.filter((x) => x !== p) : [...selected, p]
    error = ''
  }

  function next() {
    if (selected.length === 0) {
      error = t('takein.choose_at_least_one')
      return
    }
    // Keep the platforms in a fixed order.
    const queue = platforms.filter((p) => selected.includes(p))
    setQueue(queue)
    navigate(`/take-in/source/${queue[0]}`)
  }

  async function skip() {
    await post('/take-in/done')
    session.takeInDone = true
    navigate('/', { replace: true })
  }
</script>

<Step title={t('takein.sources_title')} body={t('takein.sources_body')}>
  <div class="grid" role="group" aria-label={t('takein.sources_title')}>
    {#each platforms as p (p)}
      {@const on = selected.includes(p)}
      <button type="button" class="source" class:on aria-pressed={on} onclick={() => toggle(p)}>
        <span class="check" aria-hidden="true">{#if on}<Icon name="check" size={14} stroke={3} />{/if}</span>
        <span class="name">{t(`platform.${p}`)}</span>
      </button>
    {/each}
  </div>
  {#if error}<p class="error-text" role="alert">{error}</p>{/if}

  {#snippet footer()}
    <button class="btn btn-lg" type="button" onclick={skip}>{t('common.skip')}</button>
    <button class="btn btn-primary btn-lg" type="button" onclick={next}>{t('common.continue')}</button>
  {/snippet}
</Step>

<style>
  .grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .source {
    display: flex;
    align-items: center;
    gap: 14px;
    height: 64px;
    padding: 0 18px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    background: var(--surface);
    color: var(--text);
    text-align: left;
  }

  .source.on {
    border-color: var(--ink);
    box-shadow: inset 0 0 0 1px var(--ink);
  }

  .check {
    width: 22px;
    height: 22px;
    flex: 0 0 22px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 1.5px solid var(--input-border);
    border-radius: 7px;
    color: var(--on-ink);
  }

  .on .check {
    border-color: var(--ink);
    background: var(--ink);
  }

  .name {
    font-size: 16px;
    font-weight: 600;
  }

  @media (max-width: 480px) {
    .grid {
      grid-template-columns: 1fr;
    }
  }
</style>
