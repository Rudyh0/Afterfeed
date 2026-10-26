<script lang="ts">
  import Icon from '../components/Icon.svelte'
  import MemoryCard from '../components/MemoryCard.svelte'
  import { get } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { formatFullDate, monthName, t } from '../lib/i18n/index.svelte'
  import { dateOnly, type Card } from '../lib/memories'
  import { navigate, type Params } from '../lib/router.svelte'
  import { hideSplash } from '../lib/splash'

  let { params = {} }: { params?: Params } = $props()

  const year = $derived(Number(params.year))
  const month = $derived(Number(params.month))
  let cards = $state<Card[] | null>(null)
  let error = $state('')

  $effect(() => {
    const path = `/years/${year}/${month}`
    let cancelled = false
    get<{ memories: Card[] }>(path)
      .then((res) => {
        if (!cancelled) cards = res.memories
      })
      .catch((err) => {
        if (!cancelled) error = errorMessage(err)
      })
      .finally(hideSplash)
    return () => {
      cancelled = true
    }
  })

  const days = $derived.by(() => {
    const out: { date: string; cards: Card[] }[] = []
    for (const c of cards ?? []) {
      const last = out[out.length - 1]
      if (last && last.date === c.date) last.cards.push(c)
      else out.push({ date: c.date, cards: [c] })
    }
    return out
  })

  function shift(n: number): string {
    const d = new Date(Date.UTC(year, month - 1 + n, 1))
    return `/years/${d.getUTCFullYear()}/${d.getUTCMonth() + 1}`
  }

  function label(n: number): string {
    const d = new Date(Date.UTC(year, month - 1 + n, 1))
    return `${monthName(d.getUTCMonth() + 1)} ${d.getUTCFullYear()}`
  }

  function onKey(e: KeyboardEvent) {
    if ((e.target as HTMLElement).closest('input, textarea, [role="menu"]')) return
    if (e.key === 'ArrowLeft') navigate(shift(-1))
    if (e.key === 'ArrowRight') navigate(shift(1))
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="page">
  <header class="head">
    <a class="eyebrow" href={`/years/${year}`}>{t('month.eyebrow', { year: String(year) })}</a>
    <div class="title-row">
      <h1>{monthName(month)} {year}</h1>
      <div class="stepper">
        <button type="button" aria-label={label(-1)} onclick={() => navigate(shift(-1))}><Icon name="chevron-left" size={20} /></button>
        <span class="divider" aria-hidden="true"></span>
        <button type="button" aria-label={label(1)} onclick={() => navigate(shift(1))}><Icon name="chevron-right" size={20} /></button>
      </div>
    </div>
    {#if cards}<p class="summary">{t('count.memories', { count: cards.length })}</p>{/if}
  </header>

  {#if error}
    <p class="error-text" role="alert">{error}</p>
  {:else if cards && cards.length === 0}
    <p class="empty">{t('years.no_memories')}</p>
  {:else}
    <div class="feed">
      {#each days as d (d.date)}
        <section class="day" aria-label={formatFullDate(dateOnly(d.date), 'UTC')}>
          <h2>{formatFullDate(dateOnly(d.date), 'UTC')}</h2>
          {#each d.cards as c (c.id)}
            <MemoryCard card={c} from={`/years/${year}/${month}`} />
          {/each}
        </section>
      {/each}
    </div>
  {/if}
</div>

<style>
  .page {
    max-width: 680px;
    margin: 0 auto;
  }

  .head {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 36px;
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
    font-size: 40px;
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

  .stepper button:hover {
    background: var(--surface-2);
  }

  .divider {
    width: 1px;
    height: 20px;
    background: var(--border-strong);
  }

  .summary,
  .empty {
    color: var(--text-3);
    font-size: 15px;
  }

  .feed {
    display: flex;
    flex-direction: column;
    gap: 40px;
  }

  .day {
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  h2 {
    font-size: 18px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  @media (max-width: 600px) {
    h1 {
      font-size: 32px;
    }
  }
</style>
