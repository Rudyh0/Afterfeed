<script lang="ts">
  import { onMount } from 'svelte'
  import Collage from '../components/Collage.svelte'
  import { get } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { t } from '../lib/i18n/index.svelte'
  import type { Params } from '../lib/router.svelte'
  import { hideSplash } from '../lib/splash'
  import { countsLine, type YearSummary } from './years'

  let {}: { params?: Params } = $props()
  let years = $state<YearSummary[] | null>(null)
  let error = $state('')

  onMount(async () => {
    try {
      const res = await get<{ years: YearSummary[] }>('/years')
      years = [...res.years].reverse()
    } catch (err) {
      error = errorMessage(err)
    } finally {
      hideSplash()
    }
  })
</script>

<div class="page">
  <header class="head">
    <h1 class="page-title">{t('nav.years')}</h1>
  </header>

  {#if error}
    <p class="error-text" role="alert">{error}</p>
  {:else if years && years.length === 0}
    <div class="empty">
      <p>{t('years.empty')}</p>
      <a class="btn" href="/take-in/sources">{t('nav.add_source')}</a>
    </div>
  {:else if years}
    <div class="grid">
      {#each years as y (y.year)}
        <a class="card year" href={`/years/${y.year}`}>
          <Collage covers={y.covers ?? []} height={190} />
          <span class="body">
            <span class="name tabular">{y.year}</span>
            <span class="sub tabular">{countsLine(y)}</span>
          </span>
        </a>
      {/each}
    </div>
  {/if}
</div>

<style>
  .page {
    max-width: 1100px;
    margin: 0 auto;
  }

  .head {
    margin-bottom: 32px;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 16px;
  }

  .year {
    display: flex;
    flex-direction: column;
    color: var(--text);
    text-decoration: none;
  }

  .year:hover {
    border-color: var(--border-strong);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 14px 16px 16px;
  }

  .name {
    font-size: 20px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  .sub {
    color: var(--text-muted);
    font-size: 13px;
  }

  .empty {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
    color: var(--text-3);
  }

  @media (max-width: 600px) {
    .grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
    }
  }
</style>
