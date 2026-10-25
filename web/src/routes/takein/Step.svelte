<script lang="ts">
  import type { Snippet } from 'svelte'

  interface Props {
    title: string
    body?: string
    eyebrow?: string
    children: Snippet
    footer?: Snippet
    wide?: boolean
  }
  let { title, body, eyebrow, children, footer, wide = false }: Props = $props()
</script>

<section class="step" class:wide>
  <div class="head">
    {#if eyebrow}<p class="eyebrow">{eyebrow}</p>{/if}
    <h1>{title}</h1>
    {#if body}<p class="body">{body}</p>{/if}
  </div>
  <div class="content">
    {@render children()}
  </div>
  {#if footer}
    <div class="footer">{@render footer()}</div>
  {/if}
</section>

<style>
  .step {
    width: 100%;
    max-width: 560px;
    display: flex;
    flex-direction: column;
    gap: 28px;
  }

  .step.wide {
    max-width: 760px;
  }

  .head {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  h1 {
    font-size: 32px;
    line-height: 1.15;
    font-weight: 600;
    letter-spacing: -0.025em;
  }

  .body {
    color: var(--text-3);
    font-size: 16px;
    line-height: 1.55;
  }

  .content {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .footer {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
  }

  @media (max-width: 600px) {
    h1 {
      font-size: 26px;
    }
  }
</style>
