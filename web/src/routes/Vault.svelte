<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import Avatar from '../components/Avatar.svelte'
  import Dialog from '../components/Dialog.svelte'
  import Icon from '../components/Icon.svelte'
  import PinPad from '../components/PinPad.svelte'
  import { ApiError, get, mediaUrl, post } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { formatNumber, t } from '../lib/i18n/index.svelte'
  import { duration, type Card } from '../lib/memories'
  import { platformLabel, yearRange, type Conversation } from '../lib/messages'
  import type { Params } from '../lib/router.svelte'
  import { hideSplash } from '../lib/splash'

  let {}: { params?: Params } = $props()

  interface Status {
    pin_set: boolean
    open: boolean
    autolock_minutes: number
    memories?: number
    conversations?: number
  }

  interface YearGroup {
    year: number
    count: number
    memories: Card[]
    all?: boolean
  }

  let status = $state<Status | null>(null)
  let error = $state('')
  let padError = $state('')
  let lockedUntil = $state(0)
  let now = $state(Date.now())
  let firstPin = $state('')
  let years = $state<YearGroup[]>([])
  let conversations = $state<Conversation[]>([])
  let resetOpen = $state(false)
  let password = $state('')
  let resetError = $state('')
  let resetBusy = $state(false)
  let lastTouch = Date.now()
  let lastActivity = Date.now()
  let ticker: ReturnType<typeof setInterval> | undefined

  const lockedFor = $derived(Math.max(0, Math.ceil((lockedUntil - now) / 1000)))

  onMount(async () => {
    await refresh()
    hideSplash()
    ticker = setInterval(checkIdle, 1000)
  })

  onDestroy(() => clearInterval(ticker))

  async function refresh() {
    try {
      status = await get<Status>('/vault')
      lastTouch = Date.now()
      if (status.open) await loadContent()
    } catch (err) {
      error = errorMessage(err)
    }
  }

  async function loadContent() {
    const [m, c] = await Promise.all([
      get<{ years: YearGroup[] }>('/vault/memories'),
      get<{ conversations: Conversation[] }>('/conversations?vault=1'),
    ])
    years = m.years
    conversations = c.conversations
  }

  // The vault locks after a while without activity. Activity on this page
  // keeps it open on the server; the page locks itself when idle.
  function checkIdle() {
    now = Date.now()
    if (!status?.open) return
    if (now - lastActivity > status.autolock_minutes * 60000) lock()
  }

  function onActivity() {
    lastActivity = Date.now()
    if (status?.open && lastActivity - lastTouch > 60000) {
      lastTouch = lastActivity
      get<Status>('/vault').then((s) => {
        if (!s.open) showLocked(s)
      }).catch(() => {})
    }
  }

  function onVisible() {
    if (document.visibilityState === 'visible' && status?.open) {
      get<Status>('/vault').then((s) => {
        if (!s.open) showLocked(s)
      }).catch(() => {})
    }
  }

  function showLocked(s: Status) {
    status = s
    years = []
    conversations = []
  }

  function rateLimit(err: unknown): boolean {
    if (err instanceof ApiError && err.code === 'auth.too_many_attempts') {
      lockedUntil = Date.now() + (err.retryAfter ?? 30) * 1000
      return true
    }
    return false
  }

  async function unlock(pin: string): Promise<boolean> {
    padError = ''
    try {
      status = await post<Status>('/vault/unlock', { pin })
      lastActivity = lastTouch = Date.now()
      await loadContent()
      return true
    } catch (err) {
      if (rateLimit(err)) padError = ''
      else if (err instanceof ApiError && err.code === 'vault.wrong_pin') padError = t('vault.wrong_pin')
      else padError = errorMessage(err)
      return false
    }
  }

  async function setPin(pin: string): Promise<boolean> {
    padError = ''
    if (!firstPin) {
      firstPin = pin
      return true
    }
    if (pin !== firstPin) {
      firstPin = ''
      padError = t('vault.mismatch')
      return false
    }
    try {
      status = await post<Status>('/vault/pin', { pin })
      firstPin = ''
      lastActivity = lastTouch = Date.now()
      await loadContent()
      return true
    } catch (err) {
      firstPin = ''
      padError = errorMessage(err)
      return false
    }
  }

  async function lock() {
    try {
      showLocked(await post<Status>('/vault/lock'))
    } catch (err) {
      error = errorMessage(err)
    }
  }

  async function reset(e: SubmitEvent) {
    e.preventDefault()
    resetBusy = true
    resetError = ''
    try {
      status = await post<Status>('/vault/reset', { password })
      resetOpen = false
      password = ''
      padError = ''
      firstPin = ''
    } catch (err) {
      resetError = errorMessage(err)
    } finally {
      resetBusy = false
    }
  }

  async function showAll(y: YearGroup) {
    try {
      const res = await get<{ memories: Card[] }>(`/vault/memories?year=${y.year}`)
      years = years.map((g) => (g.year === y.year ? { ...g, memories: res.memories, all: true } : g))
    } catch (err) {
      error = errorMessage(err)
    }
  }

  const summary = $derived.by(() => {
    if (!status?.open) return ''
    const parts: string[] = []
    if (status.memories) parts.push(t('count.memories', { count: status.memories }))
    if (status.conversations) parts.push(t('count.conversations', { count: status.conversations }))
    return parts.join(' · ')
  })
</script>

<svelte:window onpointerdown={onActivity} onkeydown={onActivity} onscroll={onActivity} />
<svelte:document onvisibilitychange={onVisible} />

{#if !status}
  {#if error}<p class="error-text" role="alert">{error}</p>{/if}
{:else if !status.pin_set}
  <div class="center">
    {#key firstPin === ''}
      <PinPad title={firstPin ? t('vault.confirm') : t('vault.set_title')} subtitle={t('vault.set_body')}
              error={padError} onsubmit={setPin} />
    {/key}
  </div>
{:else if !status.open}
  <div class="center">
    <PinPad title={t('vault.title')} subtitle={t('vault.enter_pin')} onsubmit={unlock} disabled={lockedFor > 0}
            error={lockedFor > 0 ? t('vault.locked_for', { seconds: lockedFor }) : padError}>
      {#snippet footer()}
        <button class="link forgot" type="button" onclick={() => (resetOpen = true)}>{t('vault.forgot')}</button>
      {/snippet}
    </PinPad>
  </div>
{:else}
  <div class="page">
    <header class="head">
      <div class="titles">
        <h1 class="page-title">{t('vault.title')}</h1>
        <p class="sub tabular">{summary}</p>
      </div>
      <button class="btn btn-pill" type="button" onclick={lock}><Icon name="lock" size={18} />{t('vault.lock')}</button>
    </header>

    {#if error}<p class="error-text" role="alert">{error}</p>{/if}

    {#if conversations.length > 0}
      <section aria-labelledby="v-chats">
        <h2 id="v-chats">{t('vault.conversations')}</h2>
        <div class="chats">
          {#each conversations as c (c.id)}
            <a class="chat" href={`/messages/${c.platform}/${c.id}?from=vault`}>
              <Avatar name={c.title} group={c.is_group} />
              <span class="chat-text">
                <span class="chat-title">{c.title}</span>
                <span class="chat-meta tabular">{[platformLabel(c.platform), t('count.messages', { count: c.message_count }), yearRange(c)].join(' · ')}</span>
              </span>
              <Icon name="chevron-right" size={20} />
            </a>
          {/each}
        </div>
      </section>
    {/if}

    {#each years as y (y.year)}
      <section aria-labelledby={`v-${y.year}`}>
        <div class="year-head">
          <h2 id={`v-${y.year}`} class="tabular">{y.year} <span class="n">{formatNumber(y.count)}</span></h2>
          {#if !y.all && y.count > y.memories.length}
            <button class="link" type="button" onclick={() => showAll(y)}>{t('common.show_all')}</button>
          {/if}
        </div>
        <div class="grid">
          {#each y.memories as m (m.id)}
            {@const cover = m.media[0]}
            <a class="tile" href={`/memory/${m.id}?from=/vault`} aria-label={`${m.date}${m.text ? `, ${m.text.slice(0, 80)}` : ''}`}>
              {#if cover}
                <img src={mediaUrl(cover.id, 'thumb')} alt="" loading="lazy" />
                {#if cover.kind === 'video'}<span class="badge">{duration(cover.duration) || t('kind.video')}</span>
                {:else if m.media_count > 1}<span class="badge tabular">{m.media_count}</span>{/if}
              {:else}
                <span class="text-tile">{m.text}</span>
              {/if}
            </a>
          {/each}
        </div>
      </section>
    {/each}

    {#if years.length === 0 && conversations.length === 0}
      <p class="empty">{t('vault.empty')}</p>
    {/if}
  </div>
{/if}

<Dialog open={resetOpen} title={t('vault.reset_title')} onclose={() => { resetOpen = false; resetError = '' }}>
  <form id="reset-form" class="reset" onsubmit={reset}>
    <p>{t('vault.reset_body')}</p>
    <div class="field">
      <label for="reset-password">{t('auth.password')}</label>
      <input id="reset-password" class="input" type="password" autocomplete="current-password" bind:value={password} />
    </div>
    {#if resetError}<p class="error-text" role="alert">{resetError}</p>{/if}
  </form>
  {#snippet footer()}
    <button class="btn" type="button" onclick={() => (resetOpen = false)}>{t('common.cancel')}</button>
    <button class="btn btn-primary" type="submit" form="reset-form" disabled={!password || resetBusy}>{t('vault.reset')}</button>
  {/snippet}
</Dialog>

<style>
  .center {
    min-height: calc(100vh - 136px);
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .forgot {
    margin-top: 16px;
  }

  .page {
    max-width: 1040px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: 32px;
  }

  .head {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    justify-content: space-between;
    gap: 16px;
  }

  .titles {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .page-title {
    font-size: 40px;
  }

  .sub {
    color: var(--text-3);
    font-size: 15px;
  }

  section {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  h2 {
    font-size: 18px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  .n {
    color: var(--text-muted);
    font-weight: 500;
  }

  .year-head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 12px;
  }

  .chats {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .chat {
    display: flex;
    align-items: center;
    gap: 14px;
    padding: 14px 16px;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface);
    color: var(--text);
    text-decoration: none;
  }

  .chat:hover {
    background: var(--surface-2);
  }

  .chat-text {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .chat-title {
    font-size: 15px;
    font-weight: 600;
  }

  .chat-meta {
    color: var(--text-muted);
    font-size: 13px;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 8px;
  }

  .tile {
    position: relative;
    aspect-ratio: 1;
    overflow: hidden;
    border-radius: var(--radius);
    background: var(--surface-3);
    color: var(--text);
    text-decoration: none;
  }

  .tile img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .badge {
    position: absolute;
    right: 6px;
    bottom: 6px;
    padding: 2px 7px;
    border-radius: 999px;
    background: rgba(0, 0, 0, 0.55);
    color: #fff;
    font-size: 12px;
    font-weight: 600;
  }

  .text-tile {
    display: -webkit-box;
    height: 100%;
    padding: 12px;
    overflow: hidden;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    font-size: 13px;
    line-height: 1.4;
    -webkit-line-clamp: 6;
    line-clamp: 6;
    -webkit-box-orient: vertical;
  }

  .empty {
    color: var(--text-3);
    font-size: 15px;
  }

  .reset {
    display: flex;
    flex-direction: column;
    gap: 16px;
    color: var(--text-2);
    font-size: 15px;
  }

  @media (max-width: 900px) {
    .center {
      min-height: calc(100dvh - 140px);
    }

    .page-title {
      font-size: 32px;
    }

    .grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 4px;
    }
  }
</style>
