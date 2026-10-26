<script lang="ts">
  // A full-screen viewer for photos and videos in messages.
  import { onMount, tick } from 'svelte'
  import { t } from '../lib/i18n/index.svelte'
  import { fileUrl, type MessageFile } from '../lib/messages'
  import Icon from './Icon.svelte'

  interface Props {
    items: MessageFile[]
    index: number
    caption?: string
    onclose: () => void
  }
  let { items, index, caption = '', onclose }: Props = $props()

  let i = $state(0)
  let root = $state<HTMLDivElement>()
  let touchX = 0
  const item = $derived(items[i])

  $effect(() => {
    i = index
  })

  onMount(() => {
    const prev = document.activeElement as HTMLElement | null
    const overflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    tick().then(() => root?.focus())
    return () => {
      document.body.style.overflow = overflow
      prev?.focus()
    }
  })

  function go(d: number) {
    const n = items.length
    i = (((i + d) % n) + n) % n
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') onclose()
    else if (e.key === 'ArrowLeft' && items.length > 1) go(-1)
    else if (e.key === 'ArrowRight' && items.length > 1) go(1)
    else return
    e.preventDefault()
  }
</script>

<div class="viewer" bind:this={root} role="dialog" aria-modal="true" aria-label={caption || t('messages.photo')} tabindex="-1"
     onkeydown={onKey}
     ontouchstart={(e) => (touchX = e.touches[0].clientX)}
     ontouchend={(e) => { const dx = e.changedTouches[0].clientX - touchX; if (Math.abs(dx) > 50 && items.length > 1) go(dx < 0 ? 1 : -1) }}>
  <div class="bar">
    <span class="count tabular">{#if items.length > 1}{i + 1} / {items.length}{/if}</span>
    <div class="actions">
      <a class="icon-btn" href={fileUrl(item.id, 'download')} download aria-label={t('common.download')}><Icon name="download" /></a>
      <button class="icon-btn" type="button" onclick={onclose} aria-label={t('common.close')}><Icon name="close" /></button>
    </div>
  </div>
  <div class="stage">
    {#key item.id}
      {#if item.kind === 'video'}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video src={fileUrl(item.id, 'original')} poster={fileUrl(item.id, 'poster')} controls autoplay playsinline></video>
      {:else}
        <img src={fileUrl(item.id, 'preview')} alt={item.name ?? ''} />
      {/if}
    {/key}
    {#if items.length > 1}
      <button class="nav prev" type="button" onclick={() => go(-1)} aria-label={t('common.previous')}><Icon name="chevron-left" size={28} /></button>
      <button class="nav next" type="button" onclick={() => go(1)} aria-label={t('common.next')}><Icon name="chevron-right" size={28} /></button>
    {/if}
  </div>
  {#if caption}<p class="caption">{caption}</p>{/if}
</div>

<style>
  .viewer {
    position: fixed;
    inset: 0;
    z-index: 200;
    display: flex;
    flex-direction: column;
    background: var(--stage);
    color: var(--on-stage);
    outline: none;
  }

  .bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    padding: 12px 16px;
    padding-top: max(12px, env(safe-area-inset-top));
  }

  .count {
    color: var(--on-stage-muted);
    font-size: 14px;
  }

  .actions {
    display: flex;
    gap: 4px;
  }

  .icon-btn {
    width: 44px;
    height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 0;
    border-radius: var(--radius);
    background: transparent;
    color: var(--on-stage);
    cursor: pointer;
  }

  .icon-btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .stage {
    position: relative;
    flex: 1 1 auto;
    min-height: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0 16px 16px;
  }

  img,
  video {
    max-width: 100%;
    max-height: 100%;
    object-fit: contain;
    border-radius: var(--radius-sm);
  }

  .nav {
    position: absolute;
    top: 50%;
    width: 48px;
    height: 48px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin-top: -24px;
    border: 0;
    border-radius: 50%;
    background: rgba(0, 0, 0, 0.45);
    color: #fff;
    cursor: pointer;
  }

  .prev {
    left: 16px;
  }

  .next {
    right: 16px;
  }

  .caption {
    max-width: 720px;
    margin: 0 auto;
    padding: 0 20px calc(20px + env(safe-area-inset-bottom));
    color: var(--on-stage-muted);
    font-size: 14px;
    text-align: center;
  }
</style>
