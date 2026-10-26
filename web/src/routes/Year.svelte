<script lang="ts">
  import Collage from '../components/Collage.svelte'
  import Icon from '../components/Icon.svelte'
  import { get } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { monthName, t } from '../lib/i18n/index.svelte'
  import { navigate, type Params } from '../lib/router.svelte'
  import { hideSplash } from '../lib/splash'
  import { countsLine, type YearSummary } from './years'

  let { params = {} }: { params?: Params } = $props()

  interface MonthSummary {
    month: number
    memories: number
    photos: number
    videos: number
    covers: { id: string; kind: string }[]
  }

  interface YearResponse {
    year: YearSummary
    months: MonthSummary[]
    years: YearSummary[]
  }

  const year = $derived(Number(params.year))
  let data = $state<YearResponse | null>(null)
  let error = $state('')

  $effect(() => {
    const y = year
    let cancelled = false
    get<YearResponse>(`/years/${y}`)
      .then((res) => {
        if (!cancelled) data = res
      })
      .catch((err) => {
        if (!cancelled) error = errorMessage(err)
      })
      .finally(hideSplash)
    return () => {
      cancelled = true
    }
  })

  const years = $derived(data?.years ?? [])
  const idx = $derived(years.findIndex((y) => y.year === year))
  const prev = $derived(idx > 0 ? years[idx - 1].year : undefined)
  const next = $derived(idx >= 0 && idx < years.length - 1 ? years[idx + 1].year : undefined)
  const maxCount = $derived(Math.max(1, ...years.map((y) => y.memories)))

  function onKey(e: KeyboardEvent) {
    if ((e.target as HTMLElement).closest('input, textarea, [role="menu"]')) return
    if (e.key === 'ArrowLeft' && prev) navigate(`/years/${prev}`)
    if (e.key === 'ArrowRight' && next) navigate(`/years/${next}`)
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="page">
  <header class="head">
    <div class="title">
      <a class="eyebrow" href="/years">{t('years.eyebrow')}</a>
      <div class="title-row">
        <h1 class="tabular">{year}</h1>
        <div class="stepper">
          <button type="button" disabled={!prev} aria-label={prev ? t('years.previous', { year: String(prev) }) : undefined} onclick={() => prev && navigate(`/years/${prev}`)}>
            <Icon name="chevron-left" size={20} />
          </button>
          <span class="divider" aria-hidden="true"></span>
          <button type="button" disabled={!next} aria-label={next ? t('years.next', { year: String(next) }) : undefined} onclick={() => next && navigate(`/years/${next}`)}>
            <Icon name="chevron-right" size={20} />
          </button>
        </div>
      </div>
      {#if data}<p class="summary tabular">{countsLine(data.year)}</p>{/if}
    </div>
  </header>

  {#if error}
    <p class="error-text" role="alert">{error}</p>
  {:else if data}
    {#if years.length > 1}
      <nav class="card strip" aria-label={t('years.all_years')}>
        <div class="strip-inner">
          {#each years as y (y.year)}
            <a href={`/years/${y.year}`} aria-current={y.year === year ? 'page' : undefined} class="tabular">
              <span class="bar-wrap" aria-hidden="true"><span class="bar" style:height={`${Math.max(6, Math.round((y.memories / maxCount) * 44))}px`}></span></span>
              {y.year}
            </a>
          {/each}
        </div>
      </nav>
    {/if}

    <div class="months">
      {#each data.months as m (m.month)}
        {#if m.memories > 0}
          <a class="card month" href={`/years/${year}/${m.month}`}>
            <Collage covers={m.covers} />
            <span class="month-body">
              <span class="month-row"><span class="month-name">{monthName(m.month)}</span><span class="n tabular">{m.memories}</span></span>
              {#if m.photos || m.videos}<span class="sub tabular">{countsLine({ photos: m.photos, videos: m.videos })}</span>{/if}
            </span>
          </a>
        {:else}
          <div class="card month empty">
            <span class="none">{t('years.no_memories')}</span>
            <span class="month-body"><span class="month-row"><span class="month-name">{monthName(m.month)}</span><span class="n tabular">0</span></span></span>
          </div>
        {/if}
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

  .title {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .eyebrow {
    text-decoration: none;
  }

  .title-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 16px;
  }

  h1 {
    font-size: 44px;
    line-height: 1.1;
    font-weight: 600;
    letter-spacing: -0.03em;
  }

  .stepper {
    display: flex;
    align-items: center;
    overflow: hidden;
    border: 1px solid var(--border-strong);
    border-radius: 999px;
    background: var(--surface);
  }

  .stepper button {
    width: 48px;
    height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 0;
    background: transparent;
    color: var(--text);
    cursor: pointer;
  }

  .stepper button:hover:not(:disabled) {
    background: var(--surface-2);
  }

  .stepper button:disabled {
    color: var(--text-muted);
    cursor: default;
    opacity: 0.4;
  }

  .divider {
    width: 1px;
    height: 20px;
    background: var(--border-strong);
  }

  .summary {
    margin-top: 4px;
    color: var(--text-3);
    font-size: 15px;
  }

  .strip {
    margin-bottom: 40px;
    padding: 12px;
    overflow-x: auto;
  }

  .strip-inner {
    display: flex;
    justify-content: space-between;
    gap: 4px;
    min-width: max-content;
  }

  .strip a {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
    width: 52px;
    padding: 8px 0;
    border-radius: var(--radius);
    color: var(--text-muted);
    font-size: 12px;
    font-weight: 500;
    text-decoration: none;
  }

  .strip a:hover {
    background: var(--surface-2);
  }

  .strip a[aria-current='page'] {
    background: var(--surface-2);
    color: var(--text);
    font-weight: 600;
  }

  .bar-wrap {
    display: flex;
    align-items: flex-end;
    height: 44px;
  }

  .bar {
    width: 18px;
    border-radius: 4px;
    background: var(--surface-3);
  }

  [aria-current='page'] .bar {
    background: var(--accent);
  }

  .months {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 16px;
  }

  .month {
    display: flex;
    flex-direction: column;
    color: var(--text);
    text-decoration: none;
  }

  a.month:hover {
    border-color: var(--border-strong);
  }

  .month-body {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 14px 16px 16px;
  }

  .month-row {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }

  .month-name {
    font-size: 15px;
    font-weight: 600;
  }

  .n,
  .sub {
    color: var(--text-muted);
    font-size: 13px;
  }

  .empty .month-name {
    color: var(--text-3);
  }

  .none {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 170px;
    background: var(--bg);
    color: var(--text-muted);
    font-size: 13px;
  }

  @media (max-width: 600px) {
    h1 {
      font-size: 34px;
    }

    .months {
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
    }
  }
</style>
