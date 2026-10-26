<script lang="ts">
  import { mediaUrl } from '../lib/api'
  import { t } from '../lib/i18n/index.svelte'
  import { duration, linkHost, mediaSummary, placeDetail, placeLabel, sourceLabel, type Card } from '../lib/memories'
  import Icon from './Icon.svelte'
  import Menu, { type MenuItem } from './Menu.svelte'

  interface Props {
    card: Card
    /** Where the detail view returns to. */
    from?: string
    /** Show the date instead of only the time, for lists across days. */
    showDate?: string
    /** In selection mode a click selects the memory instead of opening it. */
    selectable?: boolean
    selected?: boolean
    ontoggle?: () => void
  }
  let { card, from, showDate, selectable = false, selected = false, ontoggle }: Props = $props()

  function tileClick(e: MouseEvent) {
    if (selectable) {
      e.preventDefault()
      ontoggle?.()
    }
  }

  const shown = $derived(card.media.slice(0, 4))
  const extra = $derived(card.media_count - shown.length)
  const layout = $derived(shown.length >= 4 ? 'four' : shown.length === 3 ? 'three' : shown.length === 2 ? 'two' : 'one')
  const single = $derived(shown[0])
  const ratio = $derived(single && single.width && single.height ? Math.min(1.9, Math.max(0.8, single.width / single.height)) : 4 / 3)
  const isCheckin = $derived(card.media_count === 0 && !!card.place && !card.link)
  const place = $derived(placeLabel(card.place))
  const kindLabel = $derived.by(() => {
    switch (card.kind) {
      case 'checkin':
      case 'story':
      case 'reel':
      case 'reply':
        return t(`kind.${card.kind}`)
    }
    return mediaSummary(card)
  })

  function detailHref(i: number): string {
    const q = new URLSearchParams()
    if (i > 0) q.set('i', String(i))
    if (from) q.set('from', from)
    const s = q.toString()
    return `/memory/${card.id}${s ? `?${s}` : ''}`
  }

  const downloadHref = $derived(card.media_count === 1 && single ? mediaUrl(single.id, 'download') : `/api/memories/${card.id}/download`)

  const menu = $derived.by(() => {
    const items: MenuItem[] = []
    if (card.media_count > 1) {
      items.push({ label: t('menu.download_all', { count: card.media_count }), icon: 'download', href: downloadHref, download: true })
    } else if (card.media_count === 1) {
      items.push({ label: t('menu.download'), icon: 'download', href: downloadHref, download: true })
    }
    if (card.place?.lat !== undefined && card.place?.lon !== undefined) {
      items.push({ label: t('menu.show_on_map'), icon: 'map', href: `/map?memory=${card.id}` })
    }
    items.push({ label: t('memory.about'), icon: 'info', href: detailHref(0) })
    return items
  })
</script>

<article class="card memory" class:selected>
  {#if selectable}
    <button class="select" type="button" role="checkbox" aria-checked={selected}
            aria-label={t('month.select_item', { label: card.text?.slice(0, 40) || card.date })} onclick={() => ontoggle?.()}>
      {#if selected}<Icon name="check" size={16} />{/if}
    </button>
  {/if}
  {#if shown.length > 0}
    <div class="media {layout}" style:--ratio={ratio}>
      {#each shown as m, i (m.id)}
        <a class="tile" href={detailHref(i)} onclick={tileClick} aria-label={m.kind === 'video' ? t('memory.play_video', { duration: duration(m.duration) }) : t('memory.photo_n', { n: i + 1, total: card.media_count })}>
          <img
            src={mediaUrl(m.id, 'thumb')}
            srcset={layout === 'one' ? `${mediaUrl(m.id, 'thumb')} 640w, ${mediaUrl(m.id, 'preview')} 2048w` : undefined}
            sizes={layout === 'one' ? '(max-width: 720px) 100vw, 680px' : undefined}
            alt=""
            loading="lazy"
            decoding="async"
          />
          {#if m.kind === 'video'}
            <span class="play" aria-hidden="true"><Icon name="play" size={layout === 'one' ? 26 : 20} /></span>
            {#if m.duration}<span class="badge tabular" aria-hidden="true">{duration(m.duration)}</span>{/if}
          {/if}
          {#if i === shown.length - 1 && extra > 0}
            <span class="more" aria-hidden="true">+{extra}</span>
          {/if}
        </a>
      {/each}
    </div>
  {/if}

  <div class="body" class:no-media={shown.length === 0}>
    {#if isCheckin && card.place}
      <div class="checkin">
        <span class="checkin-icon" aria-hidden="true"><Icon name="pin" size={20} /></span>
        <span class="checkin-text">
          <span class="checkin-name">{place}</span>
          {#if placeDetail(card.place)}<span class="checkin-detail">{placeDetail(card.place)}</span>{/if}
        </span>
      </div>
    {/if}
    {#if card.text}<p class="text">{card.text}</p>{/if}
    {#if card.link}
      <a class="link-block" href={card.link} target="_blank" rel="noreferrer noopener">
        <Icon name="link" size={18} />
        <span class="link-host">{linkHost(card.link)}</span>
      </a>
    {/if}
    <div class="meta-row">
      <div class="meta">
        <span class="pill" class:pill-accent={card.platform === 'afterfeed'}>{sourceLabel(card.platform)}</span>
        {#if showDate}<span>{showDate}</span>{/if}
        <span class="tabular">{card.time}</span>
        {#if kindLabel}<span>{kindLabel}</span>{/if}
        {#if place && !isCheckin}
          <span class="place"><Icon name="pin" size={14} />{place}</span>
        {/if}
        {#if card.album}
          <span class="place"><Icon name="image" size={14} />{card.album}</span>
        {/if}
      </div>
      <div class="actions">
        {#if card.media_count > 0}
          <a class="icon-btn" href={downloadHref} download aria-label={card.media_count > 1 ? t('menu.download_all', { count: card.media_count }) : t('menu.download')}>
            <Icon name="download" size={20} />
          </a>
        {/if}
        <Menu items={menu} />
      </div>
    </div>
  </div>
</article>

<style>
  .memory {
    position: relative;
    overflow: visible;
  }

  .memory.selected {
    outline: 3px solid var(--accent);
    outline-offset: -1px;
  }

  .select {
    position: absolute;
    top: 12px;
    left: 12px;
    z-index: 3;
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 2px solid #fff;
    border-radius: 50%;
    background: rgba(17, 18, 20, 0.35);
    color: #fff;
    box-shadow: 0 1px 4px rgba(17, 18, 20, 0.3);
    cursor: pointer;
  }

  .selected .select {
    border-color: var(--accent);
    background: var(--accent);
  }

  .select + .body.no-media {
    padding-left: 60px;
  }

  .media {
    display: grid;
    gap: 2px;
    overflow: hidden;
    border-radius: var(--radius-lg) var(--radius-lg) 0 0;
    background: var(--surface-2);
  }

  .media.one {
    grid-template-columns: minmax(0, 1fr);
    width: 100%;
    aspect-ratio: var(--ratio);
    max-height: 560px;
  }

  .media.two {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    height: 300px;
  }

  .media.three {
    grid-template-columns: 2fr 1fr;
    grid-template-rows: repeat(2, 168px);
  }

  .media.three .tile:first-child {
    grid-row: span 2;
  }

  .media.four {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    grid-template-rows: repeat(2, 200px);
  }

  .tile {
    position: relative;
    display: block;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    background: var(--surface-3);
  }

  .tile img {
    width: 100%;
    height: 100%;
    display: block;
    object-fit: cover;
    transition: transform 0.3s ease;
  }

  .tile:hover img {
    transform: scale(1.015);
  }

  .play {
    position: absolute;
    top: 50%;
    left: 50%;
    width: 56px;
    height: 56px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin: -28px 0 0 -28px;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.92);
    color: #17181c;
  }

  .two .play,
  .three .play,
  .four .play {
    width: 44px;
    height: 44px;
    margin: -22px 0 0 -22px;
  }

  .badge {
    position: absolute;
    right: 12px;
    bottom: 12px;
    display: inline-flex;
    align-items: center;
    height: 24px;
    padding: 0 9px;
    border-radius: 999px;
    background: rgba(17, 18, 20, 0.72);
    color: #fff;
    font-size: 12px;
    font-weight: 600;
  }

  .more {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(17, 18, 20, 0.45);
    color: #fff;
    font-size: 26px;
    font-weight: 600;
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 14px 10px 8px 20px;
  }

  .body.no-media {
    padding-top: 18px;
  }

  .text {
    padding-right: 10px;
    font-size: 16px;
    line-height: 1.55;
    white-space: pre-line;
    overflow-wrap: anywhere;
  }

  .checkin {
    display: flex;
    align-items: center;
    gap: 14px;
    margin-right: 10px;
    padding: 14px;
    border-radius: var(--radius-md);
    background: var(--bg);
  }

  .checkin-icon {
    width: 44px;
    height: 44px;
    flex: 0 0 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface);
  }

  .checkin-text {
    display: flex;
    flex-direction: column;
    gap: 2px;
    min-width: 0;
  }

  .checkin-name {
    font-size: 15px;
    font-weight: 600;
  }

  .checkin-detail {
    color: var(--text-3);
    font-size: 13px;
  }

  .link-block {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    align-self: flex-start;
    max-width: calc(100% - 10px);
    min-height: 40px;
    padding: 0 14px;
    border-radius: var(--radius);
    background: var(--bg);
    color: var(--text);
    font-size: 14px;
    font-weight: 500;
    text-decoration: none;
  }

  .link-host {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .meta-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  .meta {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px 10px;
    min-width: 0;
    color: var(--text-muted);
    font-size: 13px;
  }

  .place {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .actions {
    display: flex;
    gap: 2px;
    color: var(--text-3);
  }

  @media (max-width: 600px) {
    .media.two {
      height: 220px;
    }

    .media.three {
      grid-template-rows: repeat(2, 128px);
    }

    .media.four {
      grid-template-rows: repeat(2, 150px);
    }
  }
</style>
