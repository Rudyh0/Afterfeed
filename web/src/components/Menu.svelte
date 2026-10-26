<script lang="ts" module>
  export interface MenuItem {
    label: string
    icon?: string
    href?: string
    /** Download link: the browser saves the file instead of opening it. */
    download?: boolean
    onclick?: () => void
    danger?: boolean
  }
</script>

<script lang="ts">
  import { tick } from 'svelte'
  import { t } from '../lib/i18n/index.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    items: MenuItem[]
    label?: string
    /** Light icon on a dark background. */
    onDark?: boolean
    align?: 'left' | 'right'
  }
  let { items, label = t('common.more_options'), onDark = false, align = 'right' }: Props = $props()

  let open = $state(false)
  let root: HTMLDivElement
  let list = $state<HTMLDivElement>()
  let trigger: HTMLButtonElement

  async function toggle() {
    open = !open
    if (open) {
      await tick()
      list?.querySelector<HTMLElement>('[role="menuitem"]')?.focus()
    }
  }

  function close(focusTrigger = false) {
    open = false
    if (focusTrigger) trigger?.focus()
  }

  function onKey(e: KeyboardEvent) {
    if (!open) return
    const els = [...(list?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? [])]
    const i = els.indexOf(document.activeElement as HTMLElement)
    if (e.key === 'Escape') {
      e.preventDefault()
      close(true)
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      els[(i + 1) % els.length]?.focus()
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      els[(i - 1 + els.length) % els.length]?.focus()
    } else if (e.key === 'Tab') {
      close()
    }
  }

  function onDocClick(e: MouseEvent) {
    if (open && root && !root.contains(e.target as Node)) close()
  }

  function choose(item: MenuItem) {
    close()
    item.onclick?.()
  }
</script>

<svelte:document onclick={onDocClick} />

<div class="menu" bind:this={root} onkeydown={onKey} role="presentation">
  <button
    bind:this={trigger}
    class="icon-btn"
    class:on-dark={onDark}
    type="button"
    aria-label={label}
    aria-haspopup="menu"
    aria-expanded={open}
    onclick={toggle}
  >
    <Icon name="more" size={20} />
  </button>
  {#if open}
    <div class="list" class:left={align === 'left'} role="menu" bind:this={list}>
      {#each items as item (item.label)}
        {#if item.href}
          <a
            role="menuitem"
            href={item.href}
            class:danger={item.danger}
            download={item.download ? '' : undefined}
            onclick={() => close()}
          >
            {#if item.icon}<Icon name={item.icon} size={18} />{/if}{item.label}
          </a>
        {:else}
          <button role="menuitem" type="button" class:danger={item.danger} onclick={() => choose(item)}>
            {#if item.icon}<Icon name={item.icon} size={18} />{/if}{item.label}
          </button>
        {/if}
      {/each}
    </div>
  {/if}
</div>

<style>
  .menu {
    position: relative;
  }

  .on-dark {
    color: #f4f5f7;
  }

  .on-dark:hover {
    background: rgba(255, 255, 255, 0.1);
  }

  .list {
    position: absolute;
    top: calc(100% + 6px);
    right: 0;
    z-index: 30;
    min-width: 220px;
    display: flex;
    flex-direction: column;
    padding: 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
  }

  .list.left {
    right: auto;
    left: 0;
  }

  .list a,
  .list button {
    display: flex;
    align-items: center;
    gap: 12px;
    min-height: 44px;
    padding: 0 12px;
    border: 0;
    border-radius: var(--radius);
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 14px;
    font-weight: 500;
    text-align: left;
    text-decoration: none;
    white-space: nowrap;
    cursor: pointer;
  }

  .list a:hover,
  .list button:hover,
  .list a:focus-visible,
  .list button:focus-visible {
    background: var(--surface-2);
    outline: none;
  }

  .list :global(svg) {
    color: var(--text-3);
  }

  .danger,
  .danger :global(svg) {
    color: var(--danger) !important;
  }
</style>
