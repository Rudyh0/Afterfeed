<script lang="ts">
  import { t } from '../lib/i18n/index.svelte'
  import { router } from '../lib/router.svelte'
  import { library } from '../lib/library.svelte'
  import Icon from './Icon.svelte'
  import Logo from './Logo.svelte'

  const items = [
    { href: '/', icon: 'today', label: 'nav.today', match: (p: string) => p === '/' || p.startsWith('/day/') },
    { href: '/years', icon: 'years', label: 'nav.years', match: (p: string) => p.startsWith('/years') },
    { href: '/map', icon: 'map', label: 'nav.map', match: (p: string) => p.startsWith('/map') },
    { href: '/search', icon: 'search', label: 'nav.search', match: (p: string) => p.startsWith('/search') },
    { href: '/messages', icon: 'messages', label: 'nav.messages', match: (p: string) => p.startsWith('/messages') },
    { href: '/vault', icon: 'lock', label: 'nav.vault', match: (p: string) => p.startsWith('/vault') },
  ]
</script>

<nav class="sidebar" aria-label={t('nav.main')}>
  <a class="brand" href="/" aria-label="Afterfeed">
    <Logo width={64} />
    <span>Afterfeed</span>
  </a>

  <div class="items">
    {#each items as item (item.href)}
      {@const active = item.match(router.path)}
      <a class="item" class:active href={item.href} aria-current={active ? 'page' : undefined}>
        <Icon name={active && item.icon === 'lock' ? 'lock' : item.icon} />
        {t(item.label)}
      </a>
    {/each}
  </div>

  <a class="btn btn-primary new" href="/new">
    <Icon name="plus" size={18} stroke={2} />
    {t('nav.new_memory')}
  </a>

  {#if library.importing}
    {@const imp = library.importing}
    <a class="importing" href={`/take-in/import/${imp.id}`}>
      <span class="importing-line">
        <span>{t('nav.importing', { platform: t(`platform.${imp.platform}`) })}</span>
        {#if Math.round(imp.progress * 100) > 0}<span class="tabular">{Math.round(imp.progress * 100)}%</span>{/if}
      </span>
      <span class="importing-bar"><span style:width={`${Math.max(4, Math.round(imp.progress * 100))}%`}></span></span>
    </a>
  {/if}

  {#if library.sources.length > 0}
    <div class="archive">
      <p class="label">{t('nav.archive')}</p>
      {#each library.sources as source (source.platform)}
        <div class="source">
          <span>{source.platform === 'afterfeed' ? t('nav.own_memories') : t(`platform.${source.platform}`)}</span>
          {#if source.first_year}
            <span class="range tabular">{source.first_year === source.last_year ? source.first_year : `${source.first_year}–${source.last_year}`}</span>
          {/if}
        </div>
      {/each}
      <a class="add" href="/take-in/sources">
        <Icon name="plus" size={16} stroke={2} />
        {t('nav.add_source')}
      </a>
    </div>
  {/if}

  <a class="item" class:active={router.path.startsWith('/settings')} href="/settings">
    <Icon name="settings" />
    {t('nav.settings')}
  </a>
</nav>

<style>
  .importing {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    color: var(--text-2);
    font-size: 13px;
    font-weight: 500;
    text-decoration: none;
  }

  .importing:hover {
    background: var(--surface-2);
  }

  .importing-line {
    display: flex;
    justify-content: space-between;
    gap: 8px;
  }

  .importing-bar {
    height: 4px;
    overflow: hidden;
    border-radius: 999px;
    background: var(--surface-3);
  }

  .importing-bar span {
    display: block;
    height: 100%;
    border-radius: 999px;
    background: var(--ink);
    transition: width 0.6s ease;
  }

  .sidebar {
    position: sticky;
    top: 0;
    display: flex;
    flex-direction: column;
    gap: 28px;
    width: var(--sidebar-width);
    flex: 0 0 var(--sidebar-width);
    height: 100vh;
    overflow-y: auto;
    padding: 24px 16px;
    border-right: 1px solid var(--border);
    background: var(--surface);
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 10px;
    height: 32px;
    padding: 0 8px;
    color: var(--text);
    text-decoration: none;
    font-size: 17px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  .items {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .item {
    display: flex;
    align-items: center;
    gap: 12px;
    height: 44px;
    padding: 0 12px;
    border-radius: var(--radius);
    color: var(--text-2);
    font-size: 14px;
    font-weight: 500;
    text-decoration: none;
  }

  .item :global(svg) {
    color: var(--text-muted);
  }

  .item:hover {
    background: var(--surface-2);
  }

  .item.active {
    background: var(--surface-2);
    color: var(--text);
    font-weight: 600;
  }

  .item.active :global(svg) {
    color: var(--accent);
  }

  .new {
    width: 100%;
  }

  .archive {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .label {
    margin-bottom: 6px;
    padding: 0 12px;
    color: var(--text-muted);
    font-size: 12px;
    font-weight: 600;
  }

  .source {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    height: 32px;
    padding: 0 12px;
    color: var(--text-2);
    font-size: 13px;
  }

  .range {
    color: var(--text-muted);
  }

  .add {
    display: flex;
    align-items: center;
    gap: 8px;
    height: 44px;
    padding: 0 12px;
    color: var(--accent);
    font-size: 13px;
    font-weight: 500;
    text-decoration: none;
  }

  .sidebar > .item:last-child {
    margin-top: auto;
  }
</style>
