<script lang="ts">
  import { onMount } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import Editor, { type EditableMemory } from '../components/Editor.svelte'
  import Menu, { type MenuItem } from '../components/Menu.svelte'
  import { del, get, mediaUrl, patch } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { formatBytes, formatFullDate, formatShortDate, t } from '../lib/i18n/index.svelte'
  import { loadLibrary } from '../lib/library.svelte'
  import { dateOnly, duration, formatMd, isMd, linkHost, placeDetail, placeLabel, sourceLabel, yearsAgo, type Card, type CardMedia } from '../lib/memories'
  import { back, navigate, router, setQuery, type Params } from '../lib/router.svelte'
  import { hideSplash } from '../lib/splash'

  let { params = {} }: { params?: Params } = $props()

  interface DetailMedia extends CardMedia {
    file: string
    size: number
    source?: string
  }

  interface Detail extends Omit<Card, 'media'> {
    media: DetailMedia[]
    source: { platform: string; imported_at?: string; path?: string }
    server_tz: string
    original?: EditableMemory['original']
  }

  let memory = $state<Detail | null>(null)
  let error = $state('')
  let touchX = 0

  const index = $derived(Math.min(Math.max(0, Number(router.query.get('i') ?? 0) || 0), Math.max(0, (memory?.media.length ?? 1) - 1)))
  const current = $derived(memory?.media[index])
  const from = $derived(router.query.get('from') ?? '')
  const editing = $derived(router.query.get('edit') === '1')
  const backLabel = $derived.by(() => {
    if (from === '/') return t('nav.today')
    const m = from.match(/^\/day\/(\d{2}-\d{2})$/)
    if (m && isMd(m[1])) return formatMd(m[1])
    return t('common.back')
  })

  onMount(async () => {
    try {
      memory = await get<Detail>(`/memories/${params.id}`)
    } catch (err) {
      error = errorMessage(err)
    } finally {
      hideSplash()
    }
  })

  function show(i: number) {
    if (!memory) return
    const n = memory.media.length
    const k = ((i % n) + n) % n
    setQuery({ i: k === 0 ? undefined : String(k) })
  }

  function close() {
    back(from || '/')
  }

  function onKey(e: KeyboardEvent) {
    if ((e.target as HTMLElement).closest('input, textarea, [role="menu"]')) return
    if (e.key === 'Escape') close()
    else if (e.key === 'ArrowLeft' && memory && memory.media.length > 1) show(index - 1)
    else if (e.key === 'ArrowRight' && memory && memory.media.length > 1) show(index + 1)
  }

  function onTouchStart(e: TouchEvent) {
    touchX = e.touches[0].clientX
  }

  function onTouchEnd(e: TouchEvent) {
    const dx = e.changedTouches[0].clientX - touchX
    if (Math.abs(dx) > 50 && memory && memory.media.length > 1) show(index + (dx < 0 ? 1 : -1))
  }

  function itemLabel(m: CardMedia, i: number, n: number): string {
    return m.kind === 'video' ? `${t('kind.video')} · ${t('memory.item_n', { n: i + 1, total: n })}` : t('memory.photo_n', { n: i + 1, total: n })
  }

  async function remove() {
    if (!memory || !confirm(t('memory.delete_confirm'))) return
    try {
      await del(`/memories/${memory.id}`)
      loadLibrary()
      navigate(from || '/', { replace: true })
    } catch (err) {
      error = errorMessage(err)
    }
  }

  async function setVault(v: boolean) {
    if (!memory) return
    try {
      await patch(`/memories/${memory.id}`, { vault: v })
      loadLibrary()
      if (v) navigate(from || '/', { replace: true })
      else memory = { ...memory, vault: false }
    } catch (err) {
      error = errorMessage(err)
    }
  }

  const menu = $derived.by(() => {
    if (!memory) return []
    const items: MenuItem[] = [{ label: t('menu.edit_metadata'), icon: 'pencil', onclick: () => setQuery({ edit: '1' }) }]
    if (memory.place?.lat !== undefined && memory.place?.lon !== undefined) {
      items.push({ label: t('menu.show_on_map'), icon: 'map', href: `/map?memory=${memory.id}` })
    }
    if (memory.media.length > 1) {
      items.push({ label: t('menu.download_all', { count: memory.media.length }), icon: 'download', href: `/api/memories/${memory.id}/download`, download: true })
    }
    items.push(memory.vault
      ? { label: t('menu.remove_from_vault'), icon: 'unlock', onclick: () => setVault(false) }
      : { label: t('menu.move_to_vault'), icon: 'lock', onclick: () => setVault(true) })
    if (memory.platform === 'afterfeed') {
      items.push({ label: t('memory.delete'), icon: 'trash', onclick: remove, danger: true })
    }
    return items
  })

  const sourceText = $derived.by(() => {
    if (!memory) return ''
    if (memory.platform === 'afterfeed') return t('memory.added_by_you')
    const when = memory.source.imported_at ? formatShortDate(memory.source.imported_at) : ''
    return t('memory.source_export', { platform: t(`platform.${memory.platform}`), date: when })
  })
</script>

<svelte:window onkeydown={onKey} />

{#if error && !memory}
  <div class="error-page">
    <p class="error-text" role="alert">{error}</p>
    <button class="btn" type="button" onclick={close}>{t('common.back')}</button>
  </div>
{:else if memory}
  <div class="detail" class:no-media={memory.media.length === 0}>
    <section class="stage" aria-label={t('memory.about')} class:empty={memory.media.length === 0}>
      <div class="topbar">
        <button class="back" type="button" onclick={close}><Icon name="chevron-left" size={18} />{backLabel}</button>
        {#if memory.media.length > 1 && current}
          <span class="counter tabular">{itemLabel(current, index, memory.media.length)}</span>
        {/if}
      </div>

      {#if current}
        <div class="viewer" role="presentation" ontouchstart={onTouchStart} ontouchend={onTouchEnd}>
          {#if memory.media.length > 1}
            <button class="nav prev" type="button" aria-label={t('memory.previous_item')} onclick={() => show(index - 1)}>
              <Icon name="chevron-left" size={22} />
            </button>
          {/if}
          <div class="frame-wrap">
            {#key current.id}
              <div class="frame" style:--r={current.width && current.height ? current.width / current.height : 4 / 3}
                   style:background-image={`url(${mediaUrl(current.id, 'thumb')})`}>
                {#if current.kind === 'video'}
                  <!-- svelte-ignore a11y_media_has_caption -->
                  <video src={mediaUrl(current.id, 'original')} poster={`/api/media/${current.id}/poster`} controls playsinline preload="metadata"></video>
                  {#if current.overlay}<img class="overlay" src={`/api/media/${current.id}/overlay`} alt="" />{/if}
                {:else}
                  <img src={mediaUrl(current.id, 'preview')} alt={current.text ?? ''} />
                {/if}
              </div>
            {/key}
          </div>
          {#if memory.media.length > 1}
            <button class="nav next" type="button" aria-label={t('memory.next_item')} onclick={() => show(index + 1)}>
              <Icon name="chevron-right" size={22} />
            </button>
          {/if}
        </div>
        {#if current.text}<p class="caption">{current.text}</p>{/if}
        {#if memory.media.length > 1}
          <div class="strip">
            {#each memory.media as m, i (m.id)}
              <button type="button" class="thumb" aria-label={itemLabel(m, i, memory.media.length)} aria-current={i === index}
                      onclick={() => show(i)}>
                <img src={mediaUrl(m.id, 'thumb')} alt="" loading="lazy" />
                {#if m.kind === 'video'}<span class="thumb-play"><Icon name="play" size={14} /></span>{/if}
              </button>
            {/each}
          </div>
        {/if}
      {/if}
    </section>

    <aside class="about" aria-label={t('memory.about')}>
      {#if editing}
        <Editor
          memory={memory}
          onsaved={(m) => (memory = { ...memory!, ...(m as unknown as Detail) })}
          oncancel={() => setQuery({ edit: undefined })}
        />
      {:else}
      <div class="about-top">
        <span class="pill" class:pill-accent={memory.platform === 'afterfeed'}>{sourceLabel(memory.platform)}</span>
        <div class="about-actions">
          {#if menu.length > 0}<Menu items={menu} />{/if}
          <button class="icon-btn" type="button" aria-label={t('common.close')} onclick={close}><Icon name="close" size={20} /></button>
        </div>
      </div>

      <div class="when">
        <h1>{formatFullDate(dateOnly(memory.date), 'UTC')}</h1>
        <p class="tabular">{memory.time} · {yearsAgo(memory.year)}</p>
      </div>

      {#if memory.text}<p class="text">{memory.text}</p>{/if}

      {#if memory.link}
        <a class="link-block" href={memory.link} target="_blank" rel="noreferrer noopener">
          <Icon name="link" size={18} /><span>{linkHost(memory.link)}</span>
        </a>
      {/if}

      {#if placeLabel(memory.place)}
        <div class="place">
          <span class="place-name"><Icon name="pin" size={18} />
            <span>{placeLabel(memory.place)}{#if placeDetail(memory.place)}<span class="place-detail">{placeDetail(memory.place)}</span>{/if}</span>
          </span>
          <a class="link" href={`/search?place=${encodeURIComponent(memory.place?.name || memory.place?.city || '')}`}>{t('memory.more_from_place')}</a>
        </div>
      {/if}

      <dl>
        <dt>{t('memory.source')}</dt><dd>{sourceText}</dd>
        {#if memory.album}<dt>{t('memory.album')}</dt><dd>{memory.album}</dd>{/if}
        {#if current}
          <dt>{t('memory.file')}</dt><dd class="file">{current.source?.split('/').pop() || current.file}</dd>
          {#if current.width && current.height}
            <dt>{t('memory.dimensions')}</dt><dd class="tabular">{current.width} × {current.height}</dd>
          {/if}
          {#if current.duration}<dt>{t('memory.duration')}</dt><dd class="tabular">{duration(current.duration)}</dd>{/if}
          <dt>{t('memory.size')}</dt><dd class="tabular">{formatBytes(current.size)}</dd>
        {/if}
      </dl>

      {#if error}<p class="error-text" role="alert">{error}</p>{/if}

      {#if current}
        <div class="downloads">
          <a class="btn btn-primary btn-lg" href={mediaUrl(current.id, 'download')} download>
            <Icon name="download" size={18} />{current.kind === 'video' ? t('memory.download_video') : t('memory.download_photo')}
          </a>
          {#if memory.media.length > 1}
            <a class="btn btn-lg" href={`/api/memories/${memory.id}/download`} download>{t('memory.download_all', { count: memory.media.length })}</a>
          {/if}
        </div>
      {/if}
      {/if}
    </aside>
  </div>
{/if}

<style>
  .detail {
    min-height: 100vh;
    display: flex;
    flex-wrap: wrap;
    background: #111214;
  }

  .stage {
    flex: 999 1 560px;
    min-width: 0;
    height: 100vh;
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding: 20px 24px 24px;
    color: #f4f5f7;
  }

  .stage.empty {
    height: auto;
  }

  .detail.no-media {
    flex-direction: column;
    align-items: center;
    background: var(--bg);
  }

  .no-media .stage {
    flex: 0 0 auto;
    width: 100%;
    max-width: 720px;
    padding-bottom: 0;
  }

  .no-media .back {
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow-sm);
  }

  .no-media .about {
    flex: 0 0 auto;
    width: calc(100% - 48px);
    max-width: 672px;
    min-height: 0;
    margin: 16px 24px 48px;
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
  }

  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .back {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    height: 44px;
    padding: 0 16px 0 10px;
    border: 0;
    border-radius: 999px;
    background: rgba(255, 255, 255, 0.08);
    color: #f4f5f7;
    font: inherit;
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
  }

  .back:hover {
    background: rgba(255, 255, 255, 0.14);
  }

  .counter {
    color: #b4b7bf;
    font-size: 14px;
  }

  .viewer {
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    align-items: center;
    gap: 16px;
  }

  .frame-wrap {
    flex: 1 1 auto;
    align-self: stretch;
    min-width: 0;
    min-height: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    container-type: size;
  }

  .frame {
    position: relative;
    width: min(100cqw, calc(100cqh * var(--r)));
    aspect-ratio: var(--r);
    overflow: hidden;
    border-radius: 6px;
    background-color: #1c1d21;
    background-position: center;
    background-size: cover;
  }

  .frame img,
  .frame video {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    display: block;
    object-fit: contain;
    background: transparent;
  }

  .frame video {
    background: #000;
  }

  .frame .overlay {
    pointer-events: none;
  }

  .nav {
    flex: 0 0 48px;
    width: 48px;
    height: 48px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 0;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.08);
    color: #f4f5f7;
    cursor: pointer;
  }

  .nav:hover {
    background: rgba(255, 255, 255, 0.16);
  }

  .caption {
    color: #d5d7dd;
    font-size: 15px;
    text-align: center;
  }

  .strip {
    display: flex;
    justify-content: center;
    gap: 8px;
    overflow-x: auto;
    padding-bottom: 2px;
  }

  .thumb {
    position: relative;
    flex: 0 0 64px;
    width: 64px;
    height: 48px;
    padding: 0;
    overflow: hidden;
    border: 2px solid transparent;
    border-radius: 8px;
    background: #2a2b30;
    opacity: 0.6;
    cursor: pointer;
  }

  .thumb[aria-current='true'] {
    border-color: #fff;
    opacity: 1;
  }

  .thumb img {
    width: 100%;
    height: 100%;
    object-fit: cover;
  }

  .thumb-play {
    position: absolute;
    right: 4px;
    bottom: 4px;
    display: flex;
    color: #fff;
  }

  .about {
    flex: 1 1 360px;
    min-width: 0;
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    gap: 22px;
    padding: 20px 28px 28px;
    background: var(--surface);
    color: var(--text);
  }

  .about-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .about-actions {
    display: flex;
    gap: 2px;
    color: var(--text-3);
  }

  .when {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  h1 {
    font-size: 26px;
    line-height: 1.2;
    font-weight: 600;
    letter-spacing: -0.02em;
  }

  .when p {
    color: var(--text-3);
    font-size: 14px;
  }

  .text {
    font-size: 17px;
    line-height: 1.6;
    white-space: pre-line;
    overflow-wrap: anywhere;
  }

  .link-block {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    align-self: flex-start;
    min-height: 40px;
    padding: 0 14px;
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text);
    font-size: 14px;
    font-weight: 500;
    text-decoration: none;
  }

  .place {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 8px 12px;
    padding: 6px 6px 6px 14px;
    border-radius: var(--radius-md);
    background: var(--bg);
  }

  .place-name {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 14px;
    font-weight: 500;
  }

  .place-detail {
    display: block;
    color: var(--text-3);
    font-weight: 400;
  }

  .place .link {
    display: inline-flex;
    align-items: center;
    min-height: 44px;
    padding: 0 10px;
    font-size: 14px;
    font-weight: 500;
  }

  dl {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: 10px 20px;
    padding-top: 20px;
    border-top: 1px solid var(--divider);
    font-size: 14px;
  }

  dt {
    color: var(--text-muted);
  }

  .file {
    overflow-wrap: anywhere;
  }

  .downloads {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin-top: auto;
  }

  .downloads .btn {
    justify-content: center;
  }

  .error-page {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
    padding: 48px;
  }

  @media (max-width: 900px) {
    .stage {
      height: 72vh;
      padding: 12px 12px 16px;
    }

    .nav {
      display: none;
    }

    .about {
      min-height: 0;
      padding: 20px 16px 32px;
    }
  }
</style>
