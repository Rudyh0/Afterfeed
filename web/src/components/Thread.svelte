<script lang="ts">
  // One conversation: its messages in pages, with go to date and search.
  import { tick, untrack } from 'svelte'
  import { get, patch } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { formatBytes, formatFullDate, formatNumber, formatShortDate, formatTime, t } from '../lib/i18n/index.svelte'
  import { linkHost } from '../lib/memories'
  import {
    fileFamily, fileType, fileUrl, isoDate, platformLabel, snippetParts, startOfDay, yearRange,
    type Conversation, type Message, type MessageFile, type MessagePage,
  } from '../lib/messages'
  import { router, setQuery } from '../lib/router.svelte'
  import Avatar from './Avatar.svelte'
  import Icon from './Icon.svelte'
  import MediaViewer from './MediaViewer.svelte'
  import Menu, { type MenuItem } from './Menu.svelte'
  import VoiceMessage from './VoiceMessage.svelte'

  interface Props {
    id: string
    /** Where the back button goes on small screens. */
    backHref: string
    backLabel: string
    /** Called after the conversation moved in or out of the vault. */
    onvault?: (c: Conversation) => void
  }
  let { id, backHref, backLabel, onvault }: Props = $props()

  interface Hit {
    id: number
    sender: string
    self: boolean
    at: number
    snippet: string
  }

  let conv = $state<Conversation | null>(null)
  let msgs = $state<Message[]>([])
  let moreBefore = $state(false)
  let moreAfter = $state(false)
  let loading = $state<'page' | 'before' | 'after' | null>(null)
  let error = $state('')
  let highlight = $state<number | null>(null)
  let scroller = $state<HTMLDivElement>()
  let viewer = $state<{ items: MessageFile[]; index: number; caption: string } | null>(null)
  let highlightTimer: ReturnType<typeof setTimeout> | undefined

  // A message that was jumped to stays marked for a moment.
  $effect(() => {
    if (highlight === null) return
    clearTimeout(highlightTimer)
    highlightTimer = setTimeout(() => (highlight = null), 2500)
  })

  let dateOpen = $state(false)
  let dateValue = $state('')
  let searchOpen = $state(false)
  let searchQ = $state('')
  let hits = $state<Hit[] | null>(null)
  let hitsOpen = $state(false)
  let searchInput = $state<HTMLInputElement>()
  let searchTimer: ReturnType<typeof setTimeout> | undefined

  // Load the conversation when it changes, at the message from ?m= when given.
  $effect(() => {
    const convId = id
    untrack(() => {
      const m = router.query.get('m')
      conv = null
      msgs = []
      error = ''
      searchOpen = false
      searchQ = ''
      hits = null
      dateOpen = false
      if (m) open({ around: m }, 'target', Number(m))
      else open({}, 'end')
      void convId
    })
  })

  async function open(params: Record<string, string>, place: 'end' | 'start' | 'target', target?: number) {
    loading = 'page'
    error = ''
    try {
      const qs = new URLSearchParams({ ...params, limit: '120' })
      const page = await get<MessagePage>(`/conversations/${id}?${qs}`)
      conv = page.conversation
      msgs = page.messages
      moreBefore = page.more_before
      moreAfter = page.more_after
      highlight = place === 'target' ? (target ?? null) : null
      await tick()
      if (!scroller) return
      if (place === 'end') scroller.scrollTop = scroller.scrollHeight
      else if (place === 'start') scroller.scrollTop = 0
      else scroller.querySelector(`[data-id="${target}"]`)?.scrollIntoView({ block: 'center' })
    } catch (err) {
      error = errorMessage(err)
    } finally {
      loading = null
    }
  }

  async function loadBefore() {
    if (loading || !moreBefore || msgs.length === 0 || !scroller) return
    loading = 'before'
    const first = msgs[0]
    try {
      const page = await get<MessagePage>(`/conversations/${id}?before=${first.at}&before_id=${first.id}&limit=120`)
      const height = scroller.scrollHeight
      const top = scroller.scrollTop
      msgs = [...page.messages, ...msgs]
      moreBefore = page.more_before
      await tick()
      scroller.scrollTop = top + (scroller.scrollHeight - height)
    } catch (err) {
      error = errorMessage(err)
    } finally {
      loading = null
    }
  }

  async function loadAfter() {
    if (loading || !moreAfter || msgs.length === 0) return
    loading = 'after'
    const last = msgs[msgs.length - 1]
    try {
      const page = await get<MessagePage>(`/conversations/${id}?after=${last.at}&after_id=${last.id}&limit=120`)
      msgs = [...msgs, ...page.messages]
      moreAfter = page.more_after
    } catch (err) {
      error = errorMessage(err)
    } finally {
      loading = null
    }
  }

  function onScroll() {
    if (!scroller || loading) return
    if (scroller.scrollTop < 400 && moreBefore) loadBefore()
    else if (scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 400 && moreAfter) loadAfter()
  }

  function latest() {
    setQuery({ m: undefined })
    open({}, 'end')
  }

  function goToDate(e: SubmitEvent) {
    e.preventDefault()
    if (!dateValue) return
    dateOpen = false
    setQuery({ m: undefined })
    open({ from: String(startOfDay(dateValue)) }, 'start')
  }

  function toggleSearch() {
    searchOpen = !searchOpen
    if (searchOpen) tick().then(() => searchInput?.focus())
    else {
      searchQ = ''
      hits = null
    }
  }

  function onSearchInput() {
    clearTimeout(searchTimer)
    const q = searchQ.trim()
    if (!q) {
      hits = null
      return
    }
    searchTimer = setTimeout(async () => {
      try {
        const res = await get<{ messages: Hit[] }>(`/conversations/${id}/search?q=${encodeURIComponent(q)}`)
        if (searchQ.trim() === q) {
          hits = res.messages
          hitsOpen = true
        }
      } catch (err) {
        error = errorMessage(err)
      }
    }, 250)
  }

  function jump(h: Hit) {
    hitsOpen = false
    setQuery({ m: String(h.id) })
    const el = scroller?.querySelector(`[data-id="${h.id}"]`)
    if (el) {
      highlight = h.id
      el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    } else {
      open({ around: String(h.id) }, 'target', h.id)
    }
  }

  async function setVault(v: boolean) {
    if (!conv) return
    try {
      conv = await patch<Conversation>(`/conversations/${conv.id}`, { vault: v })
      onvault?.(conv)
    } catch (err) {
      error = errorMessage(err)
    }
  }

  const menu = $derived.by((): MenuItem[] => {
    if (!conv) return []
    return [conv.vault
      ? { label: t('menu.remove_from_vault'), icon: 'unlock', onclick: () => setVault(false) }
      : { label: t('menu.move_to_vault'), icon: 'lock', onclick: () => setVault(true) }]
  })

  // Rows to render: day separators and messages, with whether each message
  // starts or ends a run of messages from the same person.
  type Row =
    | { type: 'day'; key: string; label: string }
    | { type: 'msg'; key: string; m: Message; first: boolean; last: boolean }

  const gap = 5 * 60 * 1000
  const isEvent = (m: Message) => m.kind === 'call'

  const rows = $derived.by(() => {
    const out: Row[] = []
    let day = ''
    msgs.forEach((m, i) => {
      const d = isoDate(m.at)
      if (d !== day) {
        day = d
        out.push({ type: 'day', key: `d${d}`, label: formatFullDate(new Date(m.at)) })
      }
      const prev = msgs[i - 1]
      const next = msgs[i + 1]
      const joins = (a?: Message, b?: Message) =>
        !!a && !!b && a.sender === b.sender && a.self === b.self && Math.abs(b.at - a.at) < gap &&
        isoDate(a.at) === isoDate(b.at) && !isEvent(a) && !isEvent(b)
      out.push({ type: 'msg', key: `m${m.id}`, m, first: !joins(prev, m), last: !joins(m, next) })
    })
    return out
  })

  // All photos and videos that are loaded, so the viewer can move between them.
  const visual = $derived(msgs.flatMap((m) => (m.kind === 'sticker' ? [] : (m.files ?? []).filter((f) => f.kind === 'photo' || f.kind === 'video'))))

  function view(f: MessageFile, m: Message) {
    viewer = { items: visual, index: Math.max(0, visual.findIndex((x) => x.id === f.id)), caption: m.text ?? '' }
  }

  function linkParts(text: string): { s: string; url: boolean }[] {
    return text.split(/(https?:\/\/[^\s<>"]+[^\s<>".,;:!?)\]'])/g).filter(Boolean).map((s) => ({ s, url: /^https?:\/\//.test(s) }))
  }

  function reactionSummary(m: Message): { emoji: string; count: number; who: string }[] {
    const by = new Map<string, string[]>()
    for (const r of m.reactions ?? []) {
      // A lone symbol such as ❤ shows as a black glyph without the emoji variation selector.
      const emoji = /^[\u2600-\u27bf]$/.test(r.reaction) ? r.reaction + '\ufe0f' : r.reaction
      by.set(emoji, [...(by.get(emoji) ?? []), r.actor ?? ''])
    }
    return [...by].map(([emoji, who]) => ({ emoji, count: who.length, who: who.filter(Boolean).join(', ') }))
  }

  function photoStyle(f: MessageFile): string {
    if (!f.width || !f.height) return ''
    return `aspect-ratio: ${f.width} / ${f.height}`
  }

  const subtitle = $derived(conv ? [platformLabel(conv.platform), t('count.messages', { count: conv.message_count }), yearRange(conv)].filter(Boolean).join(' · ') : '')
  const minDate = $derived(conv?.first_at ? isoDate(conv.first_at) : '')
  const maxDate = $derived(conv?.last_at ? isoDate(conv.last_at) : '')
</script>

<section class="thread" aria-labelledby="thread-title">
  <header class="head">
    <a class="icon-btn back" href={backHref} aria-label={backLabel}><Icon name="chevron-left" /></a>
    {#if conv}
      <Avatar name={conv.title} group={conv.is_group} size={40} />
      <div class="who">
        <h2 id="thread-title" title={conv.is_group ? conv.participants.join(', ') : undefined}>{conv.title}</h2>
        <p class="tabular">{subtitle}</p>
      </div>
      <div class="tools">
        <div class="date-wrap">
          <button class="btn" type="button" aria-expanded={dateOpen} onclick={() => (dateOpen = !dateOpen)}>
            <Icon name="calendar" size={18} /><span class="label">{t('messages.go_to_date')}</span>
          </button>
          {#if dateOpen}
            <form class="popover" onsubmit={goToDate}>
              <label class="visually-hidden" for="thread-date">{t('messages.go_to_date')}</label>
              <input id="thread-date" class="input" type="date" min={minDate} max={maxDate} bind:value={dateValue} />
              <button class="btn btn-primary" type="submit" disabled={!dateValue}>{t('messages.go')}</button>
            </form>
          {/if}
        </div>
        <button class="btn square" type="button" aria-label={t('messages.search_in')} aria-pressed={searchOpen} onclick={toggleSearch}>
          <Icon name={searchOpen ? 'close' : 'search'} size={18} />
        </button>
        <div class="menu-wrap"><Menu items={menu} /></div>
      </div>
    {:else}
      <h2 id="thread-title" class="visually-hidden">{t('messages.title')}</h2>
    {/if}
  </header>

  {#if searchOpen}
    <div class="search-wrap">
    <div class="searchbar">
      <Icon name="search" size={18} />
      <label class="visually-hidden" for="thread-search">{t('messages.search_in')}</label>
      <input id="thread-search" bind:this={searchInput} bind:value={searchQ} oninput={onSearchInput}
             onfocus={() => (hitsOpen = !!hits)} type="search" placeholder={t('messages.search_in')} autocomplete="off" />
      {#if hits}<span class="hit-count tabular">{t('messages.found', { count: hits.length })}</span>{/if}
    </div>
    {#if hits && hitsOpen}
      <div class="hits">
        {#if hits.length === 0}
          <p class="empty">{t('messages.no_results')}</p>
        {:else}
          <ul>
            {#each hits as h (h.id)}
              <li>
                <button type="button" onclick={() => jump(h)}>
                  <span class="hit-meta tabular">{h.self ? t('messages.you') : h.sender} · {formatShortDate(new Date(h.at))}, {formatTime(new Date(h.at))}</span>
                  <span class="hit-text">{#each snippetParts(h.snippet) as part, i (i)}{#if part.hit}<mark>{part.s}</mark>{:else}{part.s}{/if}{/each}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}
    </div>
  {/if}

  <div class="scroll" bind:this={scroller} onscroll={onScroll}>
    <div class="list">
      {#if moreBefore}
        <button class="link more" type="button" onclick={loadBefore} disabled={!!loading}>{t('messages.load_earlier')}</button>
      {/if}
      {#each rows as row (row.key)}
        {#if row.type === 'day'}
          <div class="day">{row.label}</div>
        {:else}
          {@const m = row.m}
          {@const files = m.files ?? []}
          {@const visualFiles = files.filter((f) => f.kind === 'photo' || f.kind === 'video')}
          {@const audio = files.filter((f) => f.kind === 'audio')}
          {@const docs = files.filter((f) => f.kind !== 'photo' && f.kind !== 'video' && f.kind !== 'audio')}
          {#if m.kind === 'call'}
            <div class="event" data-id={m.id} class:hl={highlight === m.id}>
              <Icon name="phone" size={16} />
              <span>{m.text || t('messages.call')}</span>
              <span class="tabular">· {formatTime(new Date(m.at))}</span>
            </div>
          {:else}
            <div class="msg" class:self={m.self} class:first={row.first} class:hl={highlight === m.id} data-id={m.id}>
              {#if row.first && !m.self && conv?.is_group}
                <span class="name">{m.sender}</span>
              {/if}

              {#if m.kind === 'sticker' && visualFiles.length > 0}
                <img class="sticker" src={fileUrl(visualFiles[0].id, 'thumb')} alt={t('messages.sticker')} loading="lazy" />
              {:else if visualFiles.length > 0}
                <div class="photos" class:grid={visualFiles.length > 1}>
                  {#each visualFiles as f (f.id)}
                    <button class="photo" type="button" onclick={() => view(f, m)}
                            aria-label={f.kind === 'video' ? t('messages.play_video') : t('messages.photo')}>
                      <img src={fileUrl(f.id, 'thumb')} alt="" loading="lazy" style={visualFiles.length === 1 ? photoStyle(f) : ''} />
                      {#if f.kind === 'video'}<span class="play-badge" aria-hidden="true"><Icon name="play" size={22} /></span>{/if}
                    </button>
                  {/each}
                </div>
              {/if}

              {#each audio as f (f.id)}
                <div class="card-bubble"><VoiceMessage file={f} /></div>
              {/each}

              {#each docs as f (f.id)}
                {@const name = f.name || t('messages.attachment')}
                <div class="card-bubble doc">
                  {#if f.kind === 'document'}
                    <a class="doc-preview" href={fileUrl(f.id, 'original')} target="_blank" rel="noopener" aria-label={t('messages.open_file', { name })}>
                      <img src={fileUrl(f.id, 'thumb')} alt="" loading="lazy"
                           onerror={(e) => ((e.currentTarget as HTMLElement).parentElement!.style.display = 'none')} />
                    </a>
                  {/if}
                  <div class="doc-row">
                    <span class="badge {fileFamily(f)}" aria-hidden="true">{fileType(f).slice(0, 4)}</span>
                    <span class="doc-text">
                      <span class="doc-name">{name}</span>
                      {#if f.size}<span class="doc-meta tabular">{t('messages.file_meta', { type: fileType(f), size: formatBytes(f.size) })}</span>{/if}
                    </span>
                    <a class="icon-btn" href={fileUrl(f.id, 'download')} download aria-label={t('messages.download_file', { name })}><Icon name="download" size={20} /></a>
                  </div>
                </div>
              {/each}

              {#if m.kind === 'unsent'}
                <p class="bubble unsent">{t('messages.unsent')}</p>
              {:else if m.text || m.link}
                <div class="bubble">
                  {#if m.text}<p>{#each linkParts(m.text) as part, i (i)}{#if part.url}<a href={part.s} target="_blank" rel="noopener noreferrer">{part.s}</a>{:else}{part.s}{/if}{/each}</p>{/if}
                  {#if m.link && !(m.text ?? '').includes(m.link)}
                    <a class="shared" href={m.link} target="_blank" rel="noopener noreferrer"><Icon name="link" size={16} />{linkHost(m.link)}</a>
                  {/if}
                </div>
              {:else if files.length === 0}
                <p class="bubble unsent">{t('messages.unsupported')}</p>
              {/if}

              {#if m.reactions?.length}
                <div class="reactions">
                  {#each reactionSummary(m) as r (r.emoji)}
                    <span class="reaction" title={r.who}>{r.emoji}{#if r.count > 1}<span class="tabular">{r.count}</span>{/if}</span>
                  {/each}
                </div>
              {/if}
              {#if row.last}<span class="time tabular">{formatTime(new Date(m.at))}</span>{/if}
            </div>
          {/if}
        {/if}
      {/each}
      {#if moreAfter}
        <button class="link more" type="button" onclick={loadAfter} disabled={!!loading}>{t('messages.load_later')}</button>
      {/if}
      {#if loading === 'page'}<p class="status">{t('common.loading')}</p>{/if}
      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
      {#if conv && !loading && msgs.length === 0 && !error}<p class="status">{t('messages.no_messages')}</p>{/if}
    </div>
  </div>

  {#if moreAfter && conv}
    <button class="btn btn-pill latest" type="button" onclick={latest}><Icon name="chevron-down" size={18} />{t('messages.latest')}</button>
  {/if}
</section>

{#if viewer}
  <MediaViewer items={viewer.items} index={viewer.index} caption={viewer.caption} onclose={() => (viewer = null)} />
{/if}

<style>
  .thread {
    position: relative;
    height: 100%;
    display: flex;
    flex-direction: column;
    min-width: 0;
    background: var(--bg);
  }

  .head {
    position: relative;
    z-index: 5;
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 73px;
    padding: 14px 24px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
  }

  .back {
    display: none;
    margin-left: -10px;
  }

  .who {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  h2 {
    overflow: hidden;
    font-size: 17px;
    font-weight: 600;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .who p {
    overflow: hidden;
    color: var(--text-muted);
    font-size: 13px;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .tools {
    display: flex;
    gap: 6px;
  }

  .square {
    width: 44px;
    padding: 0;
  }

  .square[aria-pressed='true'] {
    background: var(--surface-2);
  }

  .menu-wrap :global(.icon-btn) {
    border: 1px solid var(--border-strong);
    color: var(--text);
  }

  .date-wrap {
    position: relative;
  }

  .popover {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 20;
    display: flex;
    gap: 8px;
    width: 300px;
    padding: 10px;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
  }

  .search-wrap {
    position: relative;
    z-index: 4;
  }

  .searchbar {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 0 24px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
    color: var(--text-3);
  }

  .searchbar input {
    flex: 1 1 auto;
    min-width: 0;
    height: 52px;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--text);
    font-size: 15px;
  }

  .hit-count {
    flex: 0 0 auto;
    color: var(--text-muted);
    font-size: 13px;
  }

  .hits {
    position: absolute;
    top: 100%;
    left: 0;
    right: 0;
    z-index: 4;
    max-height: min(60vh, 420px);
    overflow: auto;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
    box-shadow: var(--shadow-md);
  }

  .hits ul {
    margin: 0;
    padding: 6px;
    list-style: none;
  }

  .hits button {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 10px 14px;
    border: 0;
    border-radius: var(--radius);
    background: none;
    color: var(--text);
    text-align: left;
    cursor: pointer;
  }

  .hits button:hover {
    background: var(--surface-2);
  }

  .hit-meta {
    color: var(--text-muted);
    font-size: 12px;
  }

  .hit-text {
    font-size: 14px;
  }

  .hits .empty {
    padding: 16px 20px;
    color: var(--text-muted);
    font-size: 14px;
  }

  mark {
    padding: 0 1px;
    border-radius: 3px;
    background: var(--accent-mark);
    color: inherit;
  }

  .scroll {
    flex: 1 1 auto;
    min-height: 0;
    overflow-y: auto;
    overscroll-behavior: contain;
    padding: 24px 40px 40px;
  }

  .list {
    max-width: 760px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .more {
    align-self: center;
  }

  .day {
    align-self: center;
    margin: 16px 0 8px;
    padding: 4px 12px;
    border-radius: 999px;
    background: var(--surface-3);
    color: var(--text-2);
    font-size: 12px;
    font-weight: 600;
  }

  .event {
    align-self: center;
    display: flex;
    align-items: center;
    gap: 6px;
    margin: 6px 0;
    color: var(--text-muted);
    font-size: 13px;
  }

  .msg {
    align-self: flex-start;
    max-width: min(72%, 520px);
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
  }

  .msg.first {
    margin-top: 6px;
  }

  .msg.self {
    align-self: flex-end;
    align-items: flex-end;
  }

  .hl .bubble,
  .hl .card-bubble,
  .hl .photos,
  .hl .sticker,
  .event.hl {
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 50%, transparent);
  }

  .bubble,
  .card-bubble,
  .photos,
  .sticker,
  .event {
    transition: box-shadow 0.8s;
  }

  .photos {
    border-radius: 14px;
  }

  .event {
    padding: 2px 8px;
    border-radius: 999px;
  }

  .name {
    padding: 0 6px;
    color: var(--text-muted);
    font-size: 12px;
    font-weight: 600;
  }

  .bubble {
    max-width: 100%;
    padding: 10px 14px;
    border: 1px solid var(--border);
    border-radius: 18px 18px 18px 6px;
    background: var(--surface);
    font-size: 15px;
    line-height: 1.45;
    overflow-wrap: anywhere;
  }

  .bubble p {
    white-space: pre-wrap;
  }

  .self .bubble {
    border-color: var(--ink);
    border-radius: 18px 18px 6px 18px;
    background: var(--ink);
    color: var(--on-ink);
  }

  .bubble a {
    color: inherit;
    text-decoration: underline;
  }

  .bubble .shared {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-top: 4px;
    font-size: 14px;
    font-weight: 500;
  }

  .bubble.unsent {
    border-style: dashed;
    background: transparent;
    color: var(--text-muted);
    font-style: italic;
  }

  .self .bubble.unsent {
    border-color: var(--border-strong);
    background: transparent;
    color: var(--text-muted);
  }

  .card-bubble {
    width: 300px;
    max-width: 100%;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: 18px 18px 18px 6px;
    background: var(--surface);
  }

  .self .card-bubble {
    border-radius: 18px 18px 6px 18px;
  }

  .doc-preview {
    display: flex;
    justify-content: center;
    height: 132px;
    padding-top: 16px;
    overflow: hidden;
    background: var(--surface-3);
  }

  .doc-preview img {
    width: 170px;
    height: auto;
    align-self: flex-start;
    border-radius: 4px 4px 0 0;
    background: #fff;
    box-shadow: var(--shadow-sm);
  }

  .doc-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 4px 8px 12px;
  }

  .badge {
    flex: 0 0 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-sm);
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.02em;
  }

  .badge.pdf {
    background: var(--file-pdf);
    color: var(--file-pdf-ink);
  }

  .badge.sheet {
    background: var(--file-sheet);
    color: var(--file-sheet-ink);
  }

  .badge.text {
    background: var(--file-text);
    color: var(--file-text-ink);
  }

  .badge.slides {
    background: var(--file-slides);
    color: var(--file-slides-ink);
  }

  .doc-text {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .doc-name {
    overflow: hidden;
    font-size: 14px;
    font-weight: 600;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .doc-meta {
    color: var(--text-muted);
    font-size: 12px;
  }

  .photos {
    width: 300px;
    max-width: 100%;
  }

  .photos.grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 4px;
  }

  .photo {
    position: relative;
    display: block;
    width: 100%;
    padding: 0;
    overflow: hidden;
    border: 0;
    border-radius: 14px;
    background: var(--surface-3);
    cursor: zoom-in;
  }

  .photo img {
    display: block;
    width: 100%;
    max-height: 380px;
    object-fit: cover;
  }

  .grid .photo img {
    aspect-ratio: 1;
  }

  .play-badge {
    position: absolute;
    top: 50%;
    left: 50%;
    width: 48px;
    height: 48px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: -24px 0 0 -24px;
    border-radius: 50%;
    background: rgba(0, 0, 0, 0.5);
    color: #fff;
  }

  .sticker {
    width: 120px;
    height: 120px;
    object-fit: contain;
  }

  .reactions {
    display: flex;
    gap: 4px;
    margin-top: -2px;
  }

  .reaction {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    height: 24px;
    padding: 0 7px;
    border: 1px solid var(--border);
    border-radius: 999px;
    background: var(--surface);
    font-size: 13px;
  }

  .time {
    padding: 0 6px;
    color: var(--text-muted);
    font-size: 12px;
  }

  .status {
    align-self: center;
    padding: 24px;
    color: var(--text-muted);
    font-size: 14px;
  }

  .latest {
    position: absolute;
    right: 24px;
    bottom: 20px;
    z-index: 3;
    box-shadow: var(--shadow-md);
  }

  @media (max-width: 900px) {
    .head {
      gap: 10px;
      min-height: 64px;
      padding: 10px 12px;
    }

    .back {
      display: inline-flex;
    }

    .tools {
      gap: 2px;
    }

    .tools .btn {
      width: 44px;
      padding: 0;
      border: 0;
    }

    .tools .label {
      display: none;
    }

    .menu-wrap :global(.icon-btn) {
      border: 0;
    }

    .searchbar {
      padding: 0 16px;
    }

    .scroll {
      padding: 16px 12px 24px;
    }

    .msg {
      max-width: 84%;
    }

    .popover {
      position: fixed;
      top: 70px;
      left: 12px;
      right: 12px;
      width: auto;
    }
  }
</style>
