<script lang="ts">
  // Renders text with **bold** parts, as used for menu names in guides.
  // The text is never parsed as HTML.
  let { text }: { text: string } = $props()
  const parts = $derived(text.split(/(\*\*[^*]+\*\*)/g).filter(Boolean))
</script>

{#each parts as part, i (i)}{#if part.startsWith('**') && part.endsWith('**')}<strong>{part.slice(2, -2)}</strong>{:else}{part}{/if}{/each}
