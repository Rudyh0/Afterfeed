<script lang="ts">
  import Icon from '../components/Icon.svelte'
  import MemoryCard from '../components/MemoryCard.svelte'
  import { get } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { formatWeekday, t } from '../lib/i18n/index.svelte'
  import { addDays, dateOnly, formatMd, isMd, todayMd, yearsAgo, type Card } from '../lib/memories'
  import { navigate, type Params } from '../lib/router.svelte'
  import { hideSplash } from '../lib/splash'

  let { params = {} }: { params?: Params } = $props()

  interface DayResponse {
    date: string
    memories: Card[]
    previous_with_memories?: string
    next_with_memories?: string
  }

  const md = $derived(isMd(params.date) ? params.date : todayMd())
  const isToday = $derived(md === todayMd())
  let data = $state<DayResponse | null>(null)
  let error = $state('')
  let picker: HTMLInputElement

  const groups = $derived.by(() => {
    const out: { year: number; cards: Card[] }[] = []
    for (const c of data?.memories ?? []) {
      const last = out[out.length - 1]
      if (last && last.year === c.year) last.cards.push(c)
      else out.push({ year: c.year, cards: [c] })
    }
    return out
  })

  $effect(() => {
    const date = md
    let cancelled = false
    error = ''
    get<DayResponse>(`/day/${date}`)
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

  function go(target: string) {
    navigate(target === todayMd() ? '/' : `/day/${target}`)
  }

  function onKey(e: KeyboardEvent) {
    const el = e.target as HTMLElement
    if (el.closest('input, textarea, select, [contenteditable], [role="menu"]') || e.metaKey || e.ctrlKey || e.altKey) return
    if (e.key === 'ArrowLeft') go(addDays(md, -1))
    if (e.key === 'ArrowRight') go(addDays(md, 1))
  }

  function openPicker() {
    picker.value = `${new Date().getFullYear()}-${md}`
    try {
      picker.showPicker()
    } catch {
      picker.focus()
      picker.click()
    }
  }

  function picked() {
    if (picker.value) go(picker.value.slice(5))
  }

  function weekday(c: Card): string {
    return formatWeekday(dateOnly(c.date), 'UTC')
  }
</script>

<svelte:window onkeydown={onKey} />

<div class="today">
  <header class="head">
    <div class="title">
      <p class="eyebrow">{t('today.eyebrow')}</p>
      <div class="title-row">
        <h1>{formatMd(md)}</h1>
        <div class="stepper">
          <button type="button" aria-label={t('today.previous_day', { date: formatMd(addDays(md, -1)) })} onclick={() => go(addDays(md, -1))}>
            <Icon name="chevron-left" size={20} />
          </button>
          <span class="divider" aria-hidden="true"></span>
          <button type="button" aria-label={t('today.next_day', { date: formatMd(addDays(md, 1)) })} onclick={() => go(addDays(md, 1))}>
            <Icon name="chevron-right" size={20} />
          </button>
        </div>
      </div>
      {#if data && data.memories.length > 0}
        <p class="summary">
          {t('today.summary', { memories: t('count.memories', { count: data.memories.length }), years: t('count.years', { count: groups.length }) })}
        </p>
      {/if}
    </div>
    <div class="tools">
      {#if !isToday}
        <button class="btn btn-pill" type="button" onclick={() => go(todayMd())}>{t('today.go_today')}</button>
      {/if}
      <button class="btn btn-pill round" type="button" aria-label={t('today.pick_date')} onclick={openPicker}>
        <Icon name="calendar" size={20} />
      </button>
      <input bind:this={picker} class="picker" type="date" tabindex="-1" aria-hidden="true" onchange={picked} />
    </div>
  </header>

  {#if error}
    <p class="error-text" role="alert">{error}</p>
  {:else if data && data.memories.length === 0}
    <div class="empty">
      <p>{t('today.empty', { date: formatMd(md) })}</p>
      {#if data.previous_with_memories || data.next_with_memories}
        <div class="nav-days">
          {#if data.previous_with_memories && data.previous_with_memories !== md}
            <button class="btn btn-pill" type="button" onclick={() => go(data!.previous_with_memories!)}>
              <Icon name="chevron-left" size={18} />{formatMd(data.previous_with_memories)}
            </button>
          {/if}
          {#if data.next_with_memories && data.next_with_memories !== md && data.next_with_memories !== data.previous_with_memories}
            <button class="btn btn-pill" type="button" onclick={() => go(data!.next_with_memories!)}>
              {formatMd(data.next_with_memories)}<Icon name="chevron-right" size={18} />
            </button>
          {/if}
        </div>
      {/if}
    </div>
  {:else if data}
    <div class="layout">
      <div class="feed">
        {#each groups as g (g.year)}
          <section class="year" id={`y${g.year}`} aria-labelledby={`h${g.year}`}>
            <div class="year-head">
              <h2 id={`h${g.year}`} class="tabular">{g.year}</h2>
              <span>{yearsAgo(g.year)} · {weekday(g.cards[0])}</span>
            </div>
            {#each g.cards as c (c.id)}
              <MemoryCard card={c} from={isToday ? '/' : `/day/${md}`} />
            {/each}
          </section>
        {/each}

        <nav class="bottom-nav" aria-label={t('today.eyebrow')}>
          <button class="btn btn-pill" type="button" onclick={() => go(addDays(md, -1))}>
            <Icon name="chevron-left" size={18} />{formatMd(addDays(md, -1))}
          </button>
          <button class="btn btn-pill" type="button" onclick={() => go(addDays(md, 1))}>
            {formatMd(addDays(md, 1))}<Icon name="chevron-right" size={18} />
          </button>
        </nav>
      </div>

      {#if groups.length > 1}
        <aside class="jump" aria-label={t('today.jump_to')}>
          <p class="jump-title">{t('today.jump_to')}</p>
          {#each groups as g (g.year)}
            <a href={`#y${g.year}`} onclick={(e) => {
              e.preventDefault()
              document.getElementById(`y${g.year}`)?.scrollIntoView({ behavior: 'smooth', block: 'start' })
            }}>
              <span class="tabular">{g.year}</span><span class="n tabular">{g.cards.length}</span>
            </a>
          {/each}
        </aside>
      {/if}
    </div>
  {/if}
</div>

<style>
  .today {
    max-width: 1000px;
    margin: 0 auto;
  }

  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    justify-content: space-between;
    gap: 24px;
    margin-bottom: 44px;
  }

  .title {
    display: flex;
    flex-direction: column;
    gap: 6px;
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

  .stepper button:hover {
    background: var(--surface-2);
  }

  .stepper .divider {
    width: 1px;
    height: 20px;
    background: var(--border-strong);
  }

  .summary {
    margin-top: 4px;
    color: var(--text-3);
    font-size: 15px;
  }

  .tools {
    position: relative;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .round {
    width: 44px;
    padding: 0;
    justify-content: center;
  }

  .picker {
    position: absolute;
    right: 0;
    bottom: 0;
    width: 1px;
    height: 1px;
    opacity: 0;
    pointer-events: none;
  }

  .layout {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-start;
    gap: 48px;
  }

  .feed {
    flex: 999 1 480px;
    min-width: 0;
    max-width: 680px;
    display: flex;
    flex-direction: column;
    gap: 48px;
  }

  .year {
    display: flex;
    flex-direction: column;
    gap: 14px;
    scroll-margin-top: 24px;
  }

  .year-head {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }

  .year-head h2 {
    font-size: 22px;
    font-weight: 600;
    letter-spacing: -0.02em;
  }

  .year-head span {
    color: var(--text-muted);
    font-size: 14px;
  }

  .bottom-nav {
    display: flex;
    flex-wrap: wrap;
    justify-content: center;
    gap: 8px;
    padding-top: 32px;
    border-top: 1px solid var(--border-strong);
  }

  .jump {
    position: sticky;
    top: 24px;
    flex: 1 1 200px;
    max-width: 240px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .jump-title {
    margin-bottom: 6px;
    padding: 0 12px;
    color: var(--text-muted);
    font-size: 12px;
    font-weight: 600;
  }

  .jump a {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: 44px;
    padding: 0 12px;
    border-radius: var(--radius);
    color: var(--text);
    font-size: 14px;
    font-weight: 500;
    text-decoration: none;
  }

  .jump a:hover {
    background: var(--surface-2);
  }

  .jump .n {
    color: var(--text-muted);
    font-weight: 400;
  }

  .empty {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
    padding: 32px 0;
    color: var(--text-3);
    font-size: 16px;
  }

  .nav-days {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  @media (max-width: 900px) {
    .jump {
      display: none;
    }
  }

  @media (max-width: 600px) {
    .head {
      margin-bottom: 28px;
    }

    h1 {
      font-size: 34px;
    }

    .feed {
      gap: 36px;
    }
  }
</style>
