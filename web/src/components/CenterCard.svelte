<script lang="ts">
  import type { Snippet } from 'svelte'
  import Logo from './Logo.svelte'

  interface Props {
    children: Snippet
    wide?: boolean
    logo?: boolean
  }
  let { children, wide = false, logo = true }: Props = $props()
</script>

<div class="page">
  <div class="card" class:wide>
    {#if logo}
      <div class="logo"><Logo width={88} /></div>
    {/if}
    {@render children()}
  </div>
</div>

<style>
  .page {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    padding: 40px 16px;
  }

  .card {
    width: 100%;
    max-width: 400px;
    display: flex;
    flex-direction: column;
    gap: 20px;
    padding: 36px 32px 32px;
    border: 1px solid var(--border);
    border-radius: 24px;
    background: var(--surface);
  }

  .card.wide {
    max-width: 560px;
  }

  .logo {
    display: flex;
    justify-content: center;
    margin-bottom: 4px;
    color: var(--text);
  }

  @media (max-width: 480px) {
    .page {
      align-items: flex-start;
    }

    .card {
      padding: 28px 20px 24px;
      border: 0;
      background: transparent;
    }
  }
</style>
