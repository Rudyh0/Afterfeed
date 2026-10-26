<script lang="ts">
  import Rich from '../../components/Rich.svelte'
  import { hasKey, t, tList } from '../../lib/i18n/index.svelte'
  import Step from './Step.svelte'
  import { continuePath } from './takein.svelte'

  let { platform }: { platform: string } = $props()
  const name = $derived(t(`platform.${platform}`))
  const steps = $derived(tList(`guide.${platform}.steps`))
  const extraSteps = $derived(tList(`guide.${platform}.extra_steps`))
  const hasExtra = $derived(hasKey(`guide.${platform}.extra_title`))
</script>

<Step title={t('takein.guide_title', { platform: name })} wide>
  <p class="intro"><Rich text={t(`guide.${platform}.intro`)} /></p>

  <ol class="steps">
    {#each steps as step, i (i)}
      <li><span class="num" aria-hidden="true">{i + 1}</span><span class="text"><Rich text={step} /></span></li>
    {/each}
  </ol>

  {#if hasExtra}
    <section class="extra card">
      <h2>{t(`guide.${platform}.extra_title`)}</h2>
      <p>{t(`guide.${platform}.extra_body`)}</p>
      <ol class="steps compact">
        {#each extraSteps as step, i (i)}
          <li><span class="num" aria-hidden="true">{i + 1}</span><span class="text"><Rich text={step} /></span></li>
        {/each}
      </ol>
    </section>
  {/if}

  <section class="ready">
    <h2>{t('takein.guide_ready_title')}</h2>
    <p><Rich text={t(`guide.${platform}.ready`)} /></p>
    <p class="muted">{t('takein.guide_keep')}</p>
  </section>

  <p class="note">{t('takein.guide_menu_note')}</p>

  {#snippet footer()}
    <a class="btn btn-lg" href={`/take-in/source/${platform}`}>{t('common.back')}</a>
    <div class="right">
      <a class="link" href={continuePath(platform)}>{t('takein.guide_later')}</a>
      <a class="btn btn-primary btn-lg" href={`/take-in/source/${platform}/upload`}>{t('takein.guide_done')}</a>
    </div>
  {/snippet}
</Step>

<style>
  .intro {
    color: var(--text-2);
    font-size: 16px;
    line-height: 1.55;
  }

  .steps {
    display: flex;
    flex-direction: column;
    gap: 14px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .steps li {
    display: flex;
    gap: 14px;
    align-items: flex-start;
    font-size: 16px;
    line-height: 1.5;
  }

  .num {
    width: 28px;
    height: 28px;
    flex: 0 0 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    margin-top: -2px;
    border-radius: 50%;
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 14px;
    font-weight: 600;
    font-variant-numeric: tabular-nums;
  }

  .steps.compact li {
    font-size: 15px;
  }

  .text :global(strong) {
    font-weight: 600;
    color: var(--text);
  }

  h2 {
    font-size: 17px;
    font-weight: 600;
    letter-spacing: -0.01em;
  }

  .extra {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 20px;
  }

  .extra p,
  .ready p {
    color: var(--text-2);
    font-size: 15px;
    line-height: 1.55;
  }

  .ready {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding-top: 8px;
  }

  .ready :global(strong),
  .intro :global(strong) {
    font-weight: 600;
    color: var(--text);
  }

  .muted,
  .note {
    color: var(--text-muted) !important;
    font-size: 14px !important;
  }

  .right {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 16px;
  }

  @media (max-width: 600px) {
    .right {
      width: 100%;
      flex-direction: column-reverse;
      align-items: stretch;
      text-align: center;
    }
  }
</style>
