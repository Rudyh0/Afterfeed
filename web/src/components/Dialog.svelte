<script lang="ts">
  // A modal dialog on the native <dialog> element, which handles focus and
  // the Escape key.
  import type { Snippet } from 'svelte'

  interface Props {
    open: boolean
    title: string
    onclose: () => void
    children: Snippet
    footer?: Snippet
    width?: number
  }
  let { open, title, onclose, children, footer, width = 520 }: Props = $props()

  let dialog = $state<HTMLDialogElement>()

  $effect(() => {
    if (!dialog) return
    if (open && !dialog.open) dialog.showModal()
    else if (!open && dialog.open) dialog.close()
  })
</script>

<dialog bind:this={dialog} onclose={onclose} style:--w={`${width}px`} aria-labelledby="dialog-title"
        onclick={(e) => e.target === dialog && onclose()}>
  <div class="inner">
    <h2 id="dialog-title">{title}</h2>
    <div class="body">{@render children()}</div>
    {#if footer}<div class="footer">{@render footer()}</div>{/if}
  </div>
</dialog>

<style>
  dialog {
    width: min(var(--w), calc(100vw - 32px));
    max-height: calc(100vh - 48px);
    padding: 0;
    overflow: auto;
    border: 1px solid var(--border);
    border-radius: var(--radius-xl);
    background: var(--surface);
    color: var(--text);
    box-shadow: var(--shadow-lg);
  }

  dialog::backdrop {
    background: var(--overlay);
  }

  .inner {
    display: flex;
    flex-direction: column;
    gap: 20px;
    padding: 24px;
  }

  h2 {
    font-size: 20px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 18px;
  }

  .footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
