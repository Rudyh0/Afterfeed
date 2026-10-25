<script lang="ts">
  import { t } from '../lib/i18n/index.svelte'
  import { router } from '../lib/router.svelte'
  import Icon from './Icon.svelte'

  let moreOpen = $state(false)

  const tabs = [
    { href: '/', icon: 'today', label: 'nav.today', match: (p: string) => p === '/' || p.startsWith('/day/') },
    { href: '/years', icon: 'years', label: 'nav.years', match: (p: string) => p.startsWith('/years') },
    { href: '/map', icon: 'map', label: 'nav.map', match: (p: string) => p.startsWith('/map') },
    { href: '/search', icon: 'search', label: 'nav.search', match: (p: string) => p.startsWith('/search') },
  ]

  const more = [
    { href: '/messages', icon: 'messages', label: 'nav.messages' },
    { href: '/vault', icon: 'lock', label: 'nav.vault' },
    { href: '/new', icon: 'plus-square', label: 'nav.new_memory' },
    { href: '/settings', icon: 'settings', label: 'nav.settings' },
  ]

  const moreActive = $derived(more.some((m) => router.path.startsWith(m.href)))

  $effect(() => {
    // Close the sheet whenever the route changes.
    router.path
    moreOpen = false
  })
</script>

<nav class="tabs" aria-label={t('nav.main')}>
  {#each tabs as tab (tab.href)}
    {@const active = tab.match(router.path)}
    <a class="tab" class:active href={tab.href} aria-current={active ? 'page' : undefined}>
      <Icon name={tab.icon} size={22} />
      <span>{t(tab.label)}</span>
    </a>
  {/each}
  <button class="tab" class:active={moreActive} type="button" aria-expanded={moreOpen} onclick={() => (moreOpen = !moreOpen)}>
    <Icon name="more" size={22} />
    <span>{t('nav.more')}</span>
  </button>
</nav>

{#if moreOpen}
  <button class="scrim" type="button" aria-label={t('common.close')} onclick={() => (moreOpen = false)}></button>
  <div class="sheet">
    {#each more as item (item.href)}
      <a class="row" href={item.href}>
        <Icon name={item.icon} />
        {t(item.label)}
      </a>
    {/each}
  </div>
{/if}

<style>
  .tabs {
    position: fixed;
    left: 0;
    right: 0;
    bottom: 0;
    z-index: 50;
    display: flex;
    padding: 6px 8px calc(6px + env(safe-area-inset-bottom));
    border-top: 1px solid var(--border);
    background: var(--surface);
  }

  .tab {
    flex: 1 1 0;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 4px;
    height: 54px;
    border: 0;
    background: none;
    color: var(--text-3);
    font-size: 11px;
    font-weight: 500;
    text-decoration: none;
  }

  .tab.active {
    color: var(--accent);
    font-weight: 600;
  }

  .scrim {
    position: fixed;
    inset: 0;
    z-index: 48;
    border: 0;
    background: var(--overlay);
  }

  .sheet {
    position: fixed;
    left: 8px;
    right: 8px;
    bottom: calc(76px + env(safe-area-inset-bottom));
    z-index: 49;
    display: flex;
    flex-direction: column;
    padding: 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
  }

  .row {
    display: flex;
    align-items: center;
    gap: 12px;
    height: 48px;
    padding: 0 12px;
    border-radius: var(--radius);
    color: var(--text);
    font-size: 15px;
    font-weight: 500;
    text-decoration: none;
  }

  .row:hover {
    background: var(--surface-2);
  }

  .row :global(svg) {
    color: var(--text-3);
  }
</style>
