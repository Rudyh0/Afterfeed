<script lang="ts">
  import type { Snippet } from 'svelte'
  import MobileNav from './MobileNav.svelte'
  import Sidebar from './Sidebar.svelte'

  interface Props {
    children: Snippet
    /** Full-bleed pages such as the map have no padding and no max width. */
    bleed?: boolean
  }
  let { children, bleed = false }: Props = $props()
</script>

<div class="shell">
  <div class="desktop-nav"><Sidebar /></div>
  <main class="content" class:bleed>
    {@render children()}
  </main>
  <div class="mobile-nav"><MobileNav /></div>
</div>

<style>
  .shell {
    display: flex;
    min-height: 100vh;
  }

  .content {
    flex: 1 1 auto;
    min-width: 0;
    padding: 40px 48px 96px;
  }

  .content.bleed {
    padding: 0;
  }

  .mobile-nav {
    display: none;
  }

  @media (max-width: 900px) {
    .desktop-nav {
      display: none;
    }

    .mobile-nav {
      display: block;
    }

    .content {
      padding: 20px 16px calc(96px + env(safe-area-inset-bottom));
    }

    .content.bleed {
      padding: 0 0 calc(68px + env(safe-area-inset-bottom));
    }
  }
</style>
