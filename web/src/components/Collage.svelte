<script lang="ts">
  import { mediaUrl } from '../lib/api'
  import Icon from './Icon.svelte'

  interface Cover {
    id: string
    kind: string
  }
  let { covers, height = 170 }: { covers: Cover[]; height?: number } = $props()
  const shown = $derived(covers.slice(0, 3))
</script>

<span class="collage n{shown.length}" style:height={`${height}px`} aria-hidden="true">
  {#each shown as c, i (c.id)}
    <span class="cell" class:first={i === 0}>
      <img src={mediaUrl(c.id, 'thumb')} alt="" loading="lazy" decoding="async" />
      {#if c.kind === 'video' && i === 0}<span class="play"><Icon name="play" size={14} /></span>{/if}
    </span>
  {/each}
  {#if shown.length === 0}<span class="cell blank"><Icon name="file" size={22} /></span>{/if}
</span>

<style>
  .collage {
    display: grid;
    gap: 2px;
    overflow: hidden;
    background: var(--surface-2);
  }

  .n2 {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .n3 {
    grid-template-columns: 2fr 1fr;
    grid-template-rows: repeat(2, minmax(0, 1fr));
  }

  .n3 .first {
    grid-row: span 2;
  }

  .cell {
    position: relative;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    background: var(--surface-3);
  }

  .cell img {
    width: 100%;
    height: 100%;
    display: block;
    object-fit: cover;
  }

  .blank {
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--surface-2);
    color: var(--text-muted);
  }

  .play {
    position: absolute;
    left: 8px;
    bottom: 8px;
    display: flex;
    padding: 5px;
    border-radius: 50%;
    background: rgba(17, 18, 20, 0.6);
    color: #fff;
  }
</style>
