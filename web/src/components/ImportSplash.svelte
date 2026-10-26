<script lang="ts">
  // A full-screen view while an import runs, in the style of the opening
  // splash: the symbol keeps moving, with the progress below it.
  import { formatNumber, t } from '../lib/i18n/index.svelte'

  interface Props {
    title: string
    /** What the import is doing now. */
    phase: string
    /** Between 0 and 1, or null when unknown. */
    progress: number | null
    done?: number
    total?: number
    /** The server could not be reached on the last try. */
    offline?: boolean
    busy?: boolean
    oncancel?: () => void
  }
  let { title, phase, progress, done = 0, total = 0, offline = false, busy = false, oncancel }: Props = $props()

  const percent = $derived(progress === null ? null : Math.round(progress * 100))
</script>

<div class="import-splash" role="status" aria-live="polite">
  <div class="center">
    <svg viewBox="0 0 132 39" fill="currentColor" aria-hidden="true">
      <defs>
        <clipPath id="is-sky"><rect x="60" y="-20" width="62" height="40" /></clipPath>
      </defs>
      <rect class="beat b0" x="0" y="18" width="12" height="4" rx="2" />
      <circle class="beat b1" cx="23.4" cy="20" r="3.4" />
      <circle class="beat b2" cx="38.2" cy="20" r="3.4" />
      <rect class="line" x="49.6" y="18" width="82.4" height="4" rx="2" />
      <g clip-path="url(#is-sky)"><path class="sun" d="M73 20V18a18 18 0 0 1 36 0v2z" /></g>
      <rect class="fade r0" x="74" y="25.8" width="34" height="2.4" rx="1.2" />
      <rect class="fade r1" x="79.5" y="30.8" width="23" height="2.4" rx="1.2" />
      <rect class="fade r2" x="85.5" y="35.8" width="11" height="2.4" rx="1.2" />
    </svg>

    <h1>{title}</h1>

    <div class="progress">
      <div class="row">
        <span>{offline ? t('takein.reconnecting') : phase}</span>
        {#if !offline && total > 0}
          <span class="tabular">{t('takein.count_of', { done: formatNumber(done), total: formatNumber(total) })}</span>
        {:else if !offline && percent !== null && percent > 0}
          <span class="tabular">{percent}%</span>
        {/if}
      </div>
      <div class="bar" class:indeterminate={percent === null || offline} role="progressbar" aria-label={phase}
           aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent ?? undefined}>
        <span style:width={percent === null || offline ? undefined : `${Math.max(2, percent)}%`}></span>
      </div>
    </div>

    <p>{t('takein.close_page')}</p>
    <p>{t('takein.patience')}</p>
  </div>

  {#if oncancel}
    <button class="stop" type="button" disabled={busy} onclick={oncancel}>{t('takein.cancel_import')}</button>
  {/if}
</div>

<style>
  .import-splash {
    position: fixed;
    inset: 0;
    z-index: 100;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: 24px 24px calc(24px + env(safe-area-inset-bottom));
    background: var(--bg);
    color: var(--text);
    text-align: center;
  }

  .center {
    width: 100%;
    max-width: 440px;
    display: flex;
    flex-direction: column;
    align-items: center;
  }

  svg {
    width: 200px;
    height: auto;
    overflow: visible;
    margin-bottom: 36px;
  }

  h1 {
    font-size: 24px;
    line-height: 1.25;
    font-weight: 600;
    letter-spacing: -0.02em;
  }

  .progress {
    width: 100%;
    margin: 28px 0 24px;
  }

  .row {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 8px;
    color: var(--text-2);
    font-size: 14px;
    font-weight: 500;
    text-align: left;
  }

  .bar {
    position: relative;
    height: 6px;
    overflow: hidden;
    border-radius: 999px;
    background: var(--surface-3);
  }

  .bar span {
    position: absolute;
    inset: 0 auto 0 0;
    border-radius: 999px;
    background: var(--ink);
    transition: width 0.6s ease;
  }

  .bar.indeterminate span {
    width: 30%;
    animation: slide 1.6s ease-in-out infinite;
  }

  p {
    max-width: 380px;
    margin-top: 8px;
    color: var(--text-3);
    font-size: 15px;
    line-height: 1.5;
  }

  .stop {
    position: absolute;
    bottom: calc(24px + env(safe-area-inset-bottom));
    min-height: 44px;
    padding: 0 16px;
    border: 0;
    border-radius: var(--radius);
    background: none;
    color: var(--text-muted);
    font-size: 14px;
    font-weight: 500;
    cursor: pointer;
  }

  .stop:hover:not(:disabled) {
    color: var(--danger);
  }

  /* The symbol draws itself like the opening splash, then the timeline keeps
     pulsing while the import runs. */
  .line {
    transform-origin: 49.6px 20px;
    transform: scaleX(0);
    animation: draw 400ms cubic-bezier(0.2, 0.8, 0.2, 1) 100ms forwards;
  }

  .sun {
    transform: translateY(20px);
    animation: rise 550ms cubic-bezier(0.2, 0.8, 0.2, 1) 400ms forwards;
  }

  .fade {
    opacity: 0;
    animation: fade 260ms ease forwards;
  }

  .r0 {
    animation-delay: 800ms;
  }

  .r1 {
    animation-delay: 870ms;
  }

  .r2 {
    animation-delay: 940ms;
  }

  .beat {
    animation: beat 1.5s ease-in-out infinite;
  }

  .b1 {
    animation-delay: 0.2s;
  }

  .b2 {
    animation-delay: 0.4s;
  }

  @keyframes draw {
    to {
      transform: scaleX(1);
    }
  }

  @keyframes rise {
    to {
      transform: translateY(0);
    }
  }

  @keyframes fade {
    to {
      opacity: 1;
    }
  }

  @keyframes beat {
    0%,
    60%,
    100% {
      opacity: 1;
    }
    30% {
      opacity: 0.2;
    }
  }

  @keyframes slide {
    from {
      left: -30%;
    }
    to {
      left: 100%;
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .line,
    .sun,
    .fade,
    .beat,
    .bar.indeterminate span {
      animation: none;
      opacity: 1;
      transform: none;
    }
  }

  @media (max-width: 600px) {
    svg {
      width: 160px;
      margin-bottom: 28px;
    }

    h1 {
      font-size: 21px;
    }
  }
</style>
