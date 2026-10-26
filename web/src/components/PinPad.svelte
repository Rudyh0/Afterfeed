<script lang="ts">
  // Six PIN dots with a keypad. The hidden input takes typed digits and is
  // what screen readers use.
  import { onMount, tick, type Snippet } from 'svelte'
  import { t } from '../lib/i18n/index.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    title: string
    subtitle: string
    error?: string
    disabled?: boolean
    /** Called with six digits; return false to clear the dots and shake. */
    onsubmit: (pin: string) => Promise<boolean>
    footer?: Snippet
  }
  let { title, subtitle, error = '', disabled = false, onsubmit, footer }: Props = $props()

  const length = 6
  let pin = $state('')
  let busy = $state(false)
  let shake = $state(false)
  let input = $state<HTMLInputElement>()

  onMount(() => {
    // On touch screens the keypad is enough; a focused input would open the
    // system keyboard as well.
    if (window.matchMedia('(pointer: fine)').matches) input?.focus()
  })

  async function set(v: string) {
    if (busy || disabled) return
    pin = v.replace(/\D/g, '').slice(0, length)
    if (pin.length === length) {
      busy = true
      const ok = await onsubmit(pin)
      busy = false
      if (!ok) {
        shake = true
        setTimeout(() => (shake = false), 400)
      }
      pin = ''
      await tick()
      if (window.matchMedia('(pointer: fine)').matches) input?.focus()
    }
  }

  function press(d: string) {
    set(pin + d)
  }
</script>

<div class="pad">
  <span class="icon" aria-hidden="true"><Icon name="lock" size={24} /></span>
  <h1>{title}</h1>
  <p class="sub">{subtitle}</p>

  <label class="visually-hidden" for="pin-input">{t('vault.pin')}</label>
  <input id="pin-input" class="visually-hidden" bind:this={input} value={pin} type="password" inputmode="numeric"
         autocomplete="off" maxlength={length} disabled={disabled}
         oninput={(e) => set((e.currentTarget as HTMLInputElement).value)} />

  <div class="dots" class:shake aria-hidden="true">
    {#each Array.from({ length }) as _, i (i)}
      <span class:on={i < pin.length}></span>
    {/each}
  </div>

  <p class="error" role="alert">{error}</p>

  <div class="keys">
    {#each ['1', '2', '3', '4', '5', '6', '7', '8', '9'] as d (d)}
      <button type="button" class="tabular" onclick={() => press(d)} disabled={disabled || busy} tabindex="-1">{d}</button>
    {/each}
    <span></span>
    <button type="button" class="tabular" onclick={() => press('0')} disabled={disabled || busy} tabindex="-1">0</button>
    <button type="button" class="del" onclick={() => (pin = pin.slice(0, -1))} disabled={disabled || busy || pin.length === 0}
            aria-label={t('vault.delete_digit')} tabindex="-1">
      <Icon name="backspace" size={24} />
    </button>
  </div>

  {#if footer}{@render footer()}{/if}
</div>

<style>
  .pad {
    width: 100%;
    max-width: 380px;
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 40px 32px 28px;
    border: 1px solid var(--border);
    border-radius: 24px;
    background: var(--surface);
  }

  .icon {
    width: 56px;
    height: 56px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin-bottom: 16px;
    border-radius: 50%;
    background: var(--surface-2);
    color: var(--text);
  }

  h1 {
    font-size: 24px;
    line-height: 1.2;
    font-weight: 600;
    letter-spacing: -0.02em;
    text-align: center;
  }

  .sub {
    margin-top: 6px;
    color: var(--text-3);
    font-size: 15px;
    text-align: center;
  }

  .dots {
    display: flex;
    gap: 14px;
    margin: 28px 0 8px;
  }

  .dots span {
    width: 14px;
    height: 14px;
    border: 2px solid var(--placeholder);
    border-radius: 50%;
  }

  .dots span.on {
    border-color: var(--ink);
    background: var(--ink);
  }

  .shake {
    animation: shake 0.4s;
  }

  @keyframes shake {
    20%,
    60% {
      transform: translateX(-8px);
    }
    40%,
    80% {
      transform: translateX(8px);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .shake {
      animation: none;
    }
  }

  .error {
    min-height: 20px;
    margin-bottom: 12px;
    color: var(--danger);
    font-size: 14px;
    text-align: center;
  }

  .keys {
    width: 100%;
    max-width: 280px;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 10px;
  }

  .keys button {
    height: 60px;
    border: 0;
    border-radius: 14px;
    background: var(--surface-2);
    color: var(--text);
    font-size: 22px;
    font-weight: 500;
    cursor: pointer;
  }

  .keys button:hover:not(:disabled) {
    background: var(--surface-3);
  }

  .keys .del {
    display: flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    color: var(--text-2);
  }

  .keys button:disabled {
    cursor: default;
  }

  .keys .del:disabled {
    opacity: 0.4;
  }
</style>
