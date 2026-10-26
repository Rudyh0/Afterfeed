<script lang="ts">
  import { onMount } from 'svelte'
  import Avatar from '../components/Avatar.svelte'
  import Icon from '../components/Icon.svelte'
  import Thread from '../components/Thread.svelte'
  import { get } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { t } from '../lib/i18n/index.svelte'
  import { loadLibrary } from '../lib/library.svelte'
  import { platformLabel, yearRange, type Conversation, type PlatformCount } from '../lib/messages'
  import { navigate, router, type Params } from '../lib/router.svelte'
  import { hideSplash } from '../lib/splash'

  let { params = {} }: { params?: Params } = $props()

  let platforms = $state<PlatformCount[] | null>(null)
  let list = $state<Conversation[]>([])
  let q = $state('')
  let loadingList = $state(false)
  let error = $state('')
  let timer: ReturnType<typeof setTimeout> | undefined
  let requested = ''

  const platform = $derived(params.platform ?? '')
  const fromVault = $derived(router.query.get('from') === 'vault')

  onMount(async () => {
    try {
      const res = await get<{ platforms: PlatformCount[] }>('/conversations?counts=1')
      platforms = res.platforms
    } catch (err) {
      error = errorMessage(err)
      platforms = []
    } finally {
      hideSplash()
    }
  })

  // Without a platform in the address, show the one with the most conversations.
  $effect(() => {
    if (!platform && platforms && platforms.length > 0) {
      navigate(`/messages/${platforms[0].platform}`, { replace: true })
    }
  })

  $effect(() => {
    if (platform) loadList(platform, '')
  })

  async function loadList(p: string, text: string) {
    const key = `${p}\n${text}`
    requested = key
    loadingList = true
    try {
      const res = await get<{ platforms: PlatformCount[]; conversations: Conversation[] }>(
        `/conversations?platform=${encodeURIComponent(p)}&q=${encodeURIComponent(text)}`,
      )
      if (requested !== key) return
      platforms = res.platforms
      list = res.conversations
      error = ''
    } catch (err) {
      error = errorMessage(err)
    } finally {
      if (requested === key) loadingList = false
    }
  }

  function onSearch() {
    clearTimeout(timer)
    timer = setTimeout(() => loadList(platform, q.trim()), 200)
  }

  function choosePlatform(p: string) {
    q = ''
    navigate(`/messages/${p}`)
  }

  function onVault(c: Conversation) {
    loadLibrary()
    loadList(platform, q.trim())
    if (c.vault) navigate(fromVault ? '/vault' : `/messages/${platform}`, { replace: true })
  }
</script>

<div class="messages" class:has-thread={!!params.id}>
  <aside class="pane">
    <h1>{t('messages.title')}</h1>
    {#if platforms && platforms.length > 1}
      <div class="segmented tabs" role="group" aria-label={t('messages.platforms')} style:--n={platforms.length}>
        {#each platforms as p (p.platform)}
          <button type="button" aria-pressed={p.platform === platform} onclick={() => choosePlatform(p.platform)}>{platformLabel(p.platform)}</button>
        {/each}
      </div>
    {/if}
    {#if platforms && platforms.length > 0}
      <div class="search">
        <Icon name="search" size={18} />
        <label class="visually-hidden" for="conv-search">{t('messages.search')}</label>
        <input id="conv-search" type="search" bind:value={q} oninput={onSearch} placeholder={t('messages.search')} autocomplete="off" />
      </div>
    {/if}

    <nav class="list" aria-label={platform ? platformLabel(platform) : t('messages.title')}>
      {#each list as c (c.id)}
        {@const active = c.id === params.id}
        <a class="row" class:active href={`/messages/${c.platform}/${c.id}`} aria-current={active ? 'page' : undefined}>
          <Avatar name={c.title} group={c.is_group} />
          <span class="text">
            <span class="line"><span class="title">{c.title}</span><span class="years tabular">{yearRange(c)}</span></span>
            <span class="last">{c.last_text || t('count.messages', { count: c.message_count })}</span>
          </span>
        </a>
      {/each}
    </nav>

    {#if platforms && platforms.length === 0 && !error}
      <div class="none">
        <p>{t('messages.none')}</p>
        <a class="btn" href="/take-in/sources"><Icon name="plus" size={18} />{t('nav.add_source')}</a>
      </div>
    {:else if !loadingList && platform && list.length === 0 && q.trim()}
      <p class="empty">{t('messages.no_match', { q: q.trim() })}</p>
    {/if}
    {#if error}<p class="error-text" role="alert">{error}</p>{/if}
  </aside>

  <div class="main">
    {#if params.id}
      {#key params.id}
        <Thread id={params.id}
                backHref={fromVault ? '/vault' : `/messages/${platform}`}
                backLabel={fromVault ? t('vault.title') : t('messages.back')}
                onvault={onVault} />
      {/key}
    {:else if list.length > 0}
      <div class="choose">
        <Icon name="messages" size={32} />
        <p>{t('messages.choose')}</p>
      </div>
    {/if}
  </div>
</div>

<style>
  .messages {
    display: flex;
    height: 100vh;
    height: 100dvh;
  }

  .pane {
    flex: 0 0 380px;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 24px 16px 16px;
    overflow-y: auto;
    border-right: 1px solid var(--border);
    background: var(--surface);
  }

  h1 {
    margin: 0 4px 4px;
    font-size: 24px;
    line-height: 1.2;
    font-weight: 600;
    letter-spacing: -0.02em;
  }

  .tabs {
    display: grid;
    grid-template-columns: repeat(var(--n), minmax(0, 1fr));
    flex: 0 0 auto;
  }

  .tabs button {
    padding: 0 4px;
    font-size: 13px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .search {
    flex: 0 0 auto;
    display: flex;
    align-items: center;
    gap: 8px;
    height: 44px;
    padding: 0 12px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text-3);
  }

  .search:focus-within {
    border-color: var(--accent);
    box-shadow: 0 0 0 3px color-mix(in srgb, var(--accent) 18%, transparent);
  }

  .search input {
    flex: 1 1 auto;
    min-width: 0;
    height: 40px;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--text);
    font-size: 15px;
  }

  .list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin-top: 4px;
  }

  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 12px;
    border-radius: var(--radius-md);
    color: var(--text);
    text-decoration: none;
  }

  .row:hover {
    background: var(--surface-2);
  }

  .row.active {
    background: var(--surface-2);
  }

  .text {
    flex: 1 1 auto;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .line {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 8px;
  }

  .title {
    overflow: hidden;
    font-size: 15px;
    font-weight: 600;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .years {
    flex-shrink: 0;
    color: var(--text-muted);
    font-size: 12px;
  }

  .last {
    overflow: hidden;
    color: var(--text-3);
    font-size: 13px;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .none,
  .empty {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 12px;
    padding: 8px 4px;
    color: var(--text-3);
    font-size: 14px;
  }

  .main {
    flex: 1 1 auto;
    min-width: 0;
    height: 100%;
    background: var(--bg);
  }

  .choose {
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    color: var(--text-muted);
    font-size: 15px;
  }

  @media (max-width: 1100px) {
    .pane {
      flex-basis: 320px;
    }
  }

  @media (max-width: 900px) {
    .messages {
      height: calc(100dvh - 68px - env(safe-area-inset-bottom));
    }

    .pane {
      flex: 1 1 auto;
      padding: 20px 16px;
      border-right: 0;
      background: var(--bg);
    }

    h1 {
      font-size: 32px;
      letter-spacing: -0.03em;
    }

    .main {
      display: none;
    }

    .has-thread .pane {
      display: none;
    }

    .has-thread .main {
      display: block;
    }
  }
</style>
