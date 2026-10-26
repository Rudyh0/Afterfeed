<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import { get, mediaUrl } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { formatNumber, formatShortDate, formatShortDayMonth, t } from '../lib/i18n/index.svelte'
  import { library } from '../lib/library.svelte'
  import { dateOnly, duration, placeLabel, sourceLabel, type Card } from '../lib/memories'
  import { router, setQuery, type Params } from '../lib/router.svelte'
  import { describePeriod, parseQuery } from '../lib/searchparse'
  import { hideSplash } from '../lib/splash'

  let {}: { params?: Params } = $props()

  interface MediaHit {
    id: string
    kind: string
    overlay?: boolean
    duration?: number
    memory_id: string
    date: string
    position: number
  }
  interface MessageHit {
    id: number
    conversation_id: string
    platform: string
    title: string
    sender: string
    self: boolean
    at: number
    snippet: string
  }
  interface Counts {
    all: number
    photos: number
    videos: number
    posts: number
    messages: number
    checkins: number
  }
  interface Results {
    counts: Counts
    media: MediaHit[]
    posts: Card[]
    messages: MessageHit[]
    checkins: Card[]
  }

  const types = ['all', 'photos', 'videos', 'posts', 'messages', 'checkins'] as const
  type Type = (typeof types)[number]

  let raw = $state(router.query.get('q') ?? '')
  const place = $derived(router.query.get('place') ?? '')
  const type = $derived((types as readonly string[]).includes(router.query.get('type') ?? '') ? (router.query.get('type') as Type) : 'all')
  const sort = $derived(router.query.get('sort') === 'oldest' ? 'oldest' : 'newest')
  const platform = $derived(router.query.get('platform') ?? '')
  const parsed = $derived(parseQuery(router.query.get('q') ?? ''))

  let results = $state<Results | null>(null)
  let more = $state<Results | null>(null)
  let loading = $state(false)
  let error = $state('')
  let timer: ReturnType<typeof setTimeout> | undefined
  let input: HTMLInputElement
  const PAGE = 60

  onMount(() => {
    hideSplash()
    if (!raw && !place) input?.focus()
  })

  function params(offset = 0): string {
    const p = new URLSearchParams()
    if (parsed.text) p.set('q', parsed.text)
    if (place) p.set('place', place)
    if (parsed.period?.from) p.set('from', parsed.period.from)
    if (parsed.period?.to) p.set('to', parsed.period.to)
    if (parsed.period?.months) p.set('months', parsed.period.months.join(','))
    if (platform) p.set('platform', platform)
    if (sort === 'oldest') p.set('sort', 'oldest')
    if (type !== 'all') {
      p.set('type', type)
      p.set('limit', String(PAGE))
      p.set('offset', String(offset))
    }
    return p.toString()
  }

  // Search again whenever the query in the address changes.
  $effect(() => {
    const qs = params()
    let cancelled = false
    if (!parsed.text && !place && !parsed.period && !platform) {
      results = null
      return
    }
    loading = true
    error = ''
    get<Results>(`/search?${qs}`)
      .then((res) => {
        if (!cancelled) {
          results = res
          more = null
        }
      })
      .catch((err) => {
        if (!cancelled) error = errorMessage(err)
      })
      .finally(() => {
        if (!cancelled) loading = false
      })
    return () => {
      cancelled = true
    }
  })

  function onInput() {
    clearTimeout(timer)
    timer = setTimeout(() => setQuery({ q: raw.trim() || undefined }), 250)
  }

  function clear() {
    raw = ''
    setQuery({ q: undefined, place: undefined })
    input.focus()
  }

  function removePeriod() {
    if (!parsed.period) return
    const words = raw.split(/\s+/)
    const tokens = parsed.period.tokens
    const i = words.findIndex((w, k) => tokens.every((tok, j) => words[k + j] === tok))
    if (i >= 0) words.splice(i, tokens.length)
    raw = words.join(' ').trim()
    setQuery({ q: raw || undefined })
  }

  async function loadMore() {
    if (!results) return
    try {
      const res = await get<Results>(`/search?${params(shownFor)}`)
      const next = more ?? { ...results, media: [], posts: [], messages: [], checkins: [] }
      next.media.push(...res.media)
      next.posts.push(...res.posts)
      next.messages.push(...res.messages)
      next.checkins.push(...res.checkins)
      more = next
    } catch (err) {
      error = errorMessage(err)
    }
  }

  const media = $derived([...(results?.media ?? []), ...(more?.media ?? [])])
  const posts = $derived([...(results?.posts ?? []), ...(more?.posts ?? [])])
  const messages = $derived([...(results?.messages ?? []), ...(more?.messages ?? [])])
  const checkins = $derived([...(results?.checkins ?? []), ...(more?.checkins ?? [])])
  const counts = $derived(results?.counts)
  const totalFor = $derived(counts ? (type === 'all' ? 0 : counts[type]) : 0)
  const shownFor = $derived(type === 'photos' || type === 'videos' ? media.length : type === 'posts' ? posts.length : type === 'messages' ? messages.length : checkins.length)

  // Highlights the searched words in text, without HTML. Split with a
  // capturing group puts the matches at odd positions.
  function parts(text: string): { s: string; hit: boolean }[] {
    const wordsList = parsed.text.split(/\s+/).filter((w) => w.length > 1)
    if (wordsList.length === 0) return [{ s: text, hit: false }]
    const re = new RegExp(`(${wordsList.map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')).join('|')})`, 'i')
    return text
      .split(re)
      .map((s, i) => ({ s, hit: i % 2 === 1 }))
      .filter((p) => p.s)
  }

  function snippet(s: string): { s: string; hit: boolean }[] {
    const out: { s: string; hit: boolean }[] = []
    for (const piece of s.split('\u0002')) {
      const [hit, rest] = piece.includes('\u0003') ? piece.split('\u0003') : [undefined, piece]
      if (hit) out.push({ s: hit, hit: true })
      if (rest) out.push({ s: rest, hit: false })
    }
    return out
  }

  function setType(v: Type) {
    setQuery({ type: v === 'all' ? undefined : v })
  }
</script>

<div class="page">
  <h1 class="page-title">{t('search.title')}</h1>

  <div class="search-box">
    <Icon name="search" size={20} />
    <label class="visually-hidden" for="search-input">{t('search.label')}</label>
    <input
      id="search-input"
      bind:this={input}
      bind:value={raw}
      oninput={onInput}
      type="search"
      placeholder={t('search.placeholder')}
      autocomplete="off"
      spellcheck="false"
    />
    {#if raw || place}
      <button class="icon-btn" type="button" aria-label={t('search.clear')} onclick={clear}><Icon name="close" size={18} /></button>
    {/if}
  </div>

  {#if parsed.period || place}
    <div class="chips">
      {#if place}
        <span class="chip"><Icon name="pin" size={14} />{place}
          <button type="button" aria-label={t('search.remove_filter', { label: place })} onclick={() => setQuery({ place: undefined })}><Icon name="close" size={14} /></button>
        </span>
      {/if}
      {#if parsed.period}
        <span class="chip"><Icon name="calendar" size={14} />{describePeriod(parsed.period)}
          <button type="button" aria-label={t('search.remove_filter', { label: parsed.period.label })} onclick={removePeriod}><Icon name="close" size={14} /></button>
        </span>
      {/if}
    </div>
  {/if}

  {#if error}
    <p class="error-text" role="alert">{error}</p>
  {/if}

  {#if !results && !loading}
    <p class="hint">{t('search.hint')}</p>
  {:else if results && counts}
    <div class="bar">
      <div class="tabs" role="group" aria-label={t('search.type')}>
        {#each types as v (v)}
          {#if v === 'all' || counts[v] > 0}
            <button type="button" aria-pressed={type === v} onclick={() => setType(v)}>
              {t(`search.type_${v}`)}<span class="count tabular">{formatNumber(counts[v])}</span>
            </button>
          {/if}
        {/each}
      </div>
      <div class="filters">
        {#if library.sources.length > 1}
          <select class="input select" aria-label={t('search.all_sources')} value={platform}
                  onchange={(e) => setQuery({ platform: (e.currentTarget as HTMLSelectElement).value || undefined })}>
            <option value="">{t('search.all_sources')}</option>
            {#each library.sources as s (s.platform)}<option value={s.platform}>{sourceLabel(s.platform)}</option>{/each}
          </select>
        {/if}
        <select class="input select" aria-label={t('search.sort_newest')} value={sort}
                onchange={(e) => setQuery({ sort: (e.currentTarget as HTMLSelectElement).value === 'oldest' ? 'oldest' : undefined })}>
          <option value="newest">{t('search.sort_newest')}</option>
          <option value="oldest">{t('search.sort_oldest')}</option>
        </select>
      </div>
    </div>

    {#if counts.all === 0}
      <p class="hint">{t('search.no_results', { query: [raw, place].filter(Boolean).join(' ') })}</p>
    {/if}

    {#if (type === 'all' || type === 'photos' || type === 'videos') && media.length > 0}
      <section class="section">
        {#if type === 'all'}
          <div class="section-head">
            <h2>{t('search.results_media')} <span class="count tabular">{formatNumber(counts.photos + counts.videos)}</span></h2>
            {#if counts.photos + counts.videos > media.length}
              <button class="link" type="button" onclick={() => setType(counts.photos > 0 ? 'photos' : 'videos')}>{t('common.show_all')}</button>
            {/if}
          </div>
        {/if}
        <div class="grid">
          {#each media as m (m.id)}
            <a class="tile" href={`/memory/${m.memory_id}${m.position ? `?i=${m.position}` : ''}`}>
              <span class="img">
                <img src={mediaUrl(m.id, 'thumb')} alt="" loading="lazy" decoding="async" />
                {#if m.kind === 'video'}<span class="badge tabular"><Icon name="play" size={12} />{duration(m.duration)}</span>{/if}
              </span>
              <span class="date tabular">{formatShortDate(dateOnly(m.date), 'UTC')}</span>
            </a>
          {/each}
        </div>
      </section>
    {/if}

    {#if (type === 'all' || type === 'posts') && posts.length > 0}
      <section class="section">
        {#if type === 'all'}
          <div class="section-head">
            <h2>{t('search.results_posts')} <span class="count tabular">{formatNumber(counts.posts)}</span></h2>
            {#if counts.posts > posts.length}<button class="link" type="button" onclick={() => setType('posts')}>{t('common.show_all')}</button>{/if}
          </div>
        {/if}
        <ul class="card rows">
          {#each posts as p (p.id)}
            <li>
              <a class="row" href={`/memory/${p.id}`}>
                <span class="when tabular"><span>{formatShortDayMonth(dateOnly(p.date), 'UTC')}</span><span class="y">{p.year}</span></span>
                <span class="row-body">
                  <span class="row-text">{#each parts(p.text ?? '') as part, i (i)}{#if part.hit}<mark>{part.s}</mark>{:else}{part.s}{/if}{/each}</span>
                  <span class="row-meta">
                    <span class="pill" class:pill-accent={p.platform === 'afterfeed'}>{sourceLabel(p.platform)}</span>
                    {#if placeLabel(p.place)}<span class="place"><Icon name="pin" size={13} />{placeLabel(p.place)}</span>{/if}
                  </span>
                </span>
                {#if p.media[0]}<img class="row-thumb" src={mediaUrl(p.media[0].id, 'thumb')} alt="" loading="lazy" />{/if}
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if (type === 'all' || type === 'messages') && messages.length > 0}
      <section class="section">
        {#if type === 'all'}
          <div class="section-head">
            <h2>{t('search.results_messages')} <span class="count tabular">{formatNumber(counts.messages)}</span></h2>
            {#if counts.messages > messages.length}<button class="link" type="button" onclick={() => setType('messages')}>{t('common.show_all')}</button>{/if}
          </div>
        {/if}
        <ul class="card rows">
          {#each messages as m (m.id)}
            <li>
              <a class="row" href={`/messages/${m.platform}/${m.conversation_id}?m=${m.id}`}>
                <span class="avatar" aria-hidden="true">{(m.self ? t('messages.you') : m.sender || m.title).charAt(0).toUpperCase()}</span>
                <span class="row-body">
                  <span class="row-title">
                    <strong>{m.self ? t('messages.you') : m.sender}</strong>
                    <span class="muted">{m.title} · {sourceLabel(m.platform)}</span>
                    <span class="muted tabular">{formatShortDate(new Date(m.at))}</span>
                  </span>
                  <span class="row-text">{#each snippet(m.snippet) as part, i (i)}{#if part.hit}<mark>{part.s}</mark>{:else}{part.s}{/if}{/each}</span>
                </span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if (type === 'all' || type === 'checkins') && checkins.length > 0}
      <section class="section">
        {#if type === 'all'}
          <div class="section-head">
            <h2>{t('search.results_checkins')} <span class="count tabular">{formatNumber(counts.checkins)}</span></h2>
            {#if counts.checkins > checkins.length}<button class="link" type="button" onclick={() => setType('checkins')}>{t('common.show_all')}</button>{/if}
          </div>
        {/if}
        <ul class="card rows">
          {#each checkins as c (c.id)}
            <li>
              <a class="row" href={`/memory/${c.id}`}>
                <span class="avatar square" aria-hidden="true"><Icon name="pin" size={18} /></span>
                <span class="row-body">
                  <strong>{placeLabel(c.place)}</strong>
                  <span class="muted tabular">{formatShortDate(dateOnly(c.date), 'UTC')} · {sourceLabel(c.platform)}</span>
                </span>
              </a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if type !== 'all' && shownFor < totalFor}
      <div class="more"><button class="btn" type="button" onclick={loadMore}>{t('common.show_all')}</button></div>
    {/if}
  {/if}
</div>

<style>
  .page {
    max-width: 1000px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .search-box {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 56px;
    padding: 0 8px 0 18px;
    border: 1px solid var(--input-border);
    border-radius: var(--radius-lg);
    background: var(--surface);
    color: var(--text-3);
  }

  .search-box:focus-within {
    border-color: var(--text-muted);
    box-shadow: 0 0 0 3px var(--accent-soft);
  }

  .search-box input {
    flex: 1 1 auto;
    min-width: 0;
    height: 100%;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 17px;
  }

  .search-box input::-webkit-search-cancel-button {
    display: none;
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: -8px;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 34px;
    padding: 0 4px 0 12px;
    border-radius: 999px;
    background: var(--accent-soft);
    color: var(--text);
    font-size: 14px;
    font-weight: 500;
  }

  .chip button {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 0;
    border-radius: 50%;
    background: transparent;
    color: var(--text-3);
    cursor: pointer;
  }

  .chip button:hover {
    background: rgba(0, 0, 0, 0.06);
  }

  .hint {
    color: var(--text-3);
  }

  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .tabs {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .tabs button {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    height: 40px;
    padding: 0 14px;
    border: 1px solid transparent;
    border-radius: 999px;
    background: transparent;
    color: var(--text-2);
    font: inherit;
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
  }

  .tabs button:hover {
    background: var(--surface-2);
  }

  .tabs button[aria-pressed='true'] {
    border-color: var(--border-strong);
    background: var(--surface);
    color: var(--text);
    font-weight: 600;
  }

  .count {
    color: var(--text-muted);
    font-weight: 500;
  }

  .filters {
    display: flex;
    gap: 8px;
  }

  .select {
    height: 40px;
    width: auto;
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .section-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
  }

  h2 {
    font-size: 18px;
    font-weight: 600;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 12px;
  }

  .tile {
    display: flex;
    flex-direction: column;
    gap: 6px;
    color: var(--text-3);
    text-decoration: none;
  }

  .img {
    position: relative;
    aspect-ratio: 1;
    overflow: hidden;
    border-radius: var(--radius-md);
    background: var(--surface-2);
  }

  .img img {
    width: 100%;
    height: 100%;
    display: block;
    object-fit: cover;
  }

  .badge {
    position: absolute;
    right: 8px;
    bottom: 8px;
    display: inline-flex;
    align-items: center;
    gap: 3px;
    height: 22px;
    padding: 0 7px;
    border-radius: 999px;
    background: rgba(17, 18, 20, 0.72);
    color: #fff;
    font-size: 11px;
    font-weight: 600;
  }

  .date {
    font-size: 13px;
  }

  .rows {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .rows li + li {
    border-top: 1px solid var(--divider);
  }

  .row {
    display: flex;
    align-items: flex-start;
    gap: 16px;
    padding: 14px 16px;
    color: var(--text);
    text-decoration: none;
  }

  .row:hover {
    background: var(--bg);
  }

  .when {
    flex: 0 0 64px;
    display: flex;
    flex-direction: column;
    font-size: 14px;
    font-weight: 600;
  }

  .when .y {
    color: var(--text-muted);
    font-weight: 400;
  }

  .row-body {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .row-text {
    display: -webkit-box;
    overflow: hidden;
    font-size: 15px;
    line-height: 1.5;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    line-clamp: 3;
  }

  .row-title {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 4px 10px;
    font-size: 14px;
  }

  .row-meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
    color: var(--text-muted);
    font-size: 13px;
  }

  .place {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .row-thumb {
    flex: 0 0 64px;
    width: 64px;
    height: 64px;
    border-radius: var(--radius);
    object-fit: cover;
  }

  .avatar {
    flex: 0 0 40px;
    width: 40px;
    height: 40px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    background: var(--surface-2);
    color: var(--text-2);
    font-weight: 600;
  }

  .avatar.square {
    border-radius: var(--radius-md);
  }

  .muted {
    color: var(--text-muted);
    font-size: 13px;
  }

  mark {
    padding: 0 1px;
    border-radius: 3px;
    background: var(--accent-soft);
    color: inherit;
    font-weight: 600;
  }

  .more {
    display: flex;
    justify-content: center;
  }

  @media (max-width: 600px) {
    .grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 6px;
    }

    .date {
      display: none;
    }

    .filters {
      width: 100%;
    }

    .filters .select {
      flex: 1 1 0;
    }
  }
</style>
