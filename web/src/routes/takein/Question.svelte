<script lang="ts">
  import Icon from '../../components/Icon.svelte'
  import { t } from '../../lib/i18n/index.svelte'
  import Step from './Step.svelte'
  import { continuePath } from './takein.svelte'

  let { platform }: { platform: string } = $props()
  const name = $derived(t(`platform.${platform}`))
</script>

<Step title={t('takein.source_question', { platform: name })}>
  <a class="option" href={`/take-in/source/${platform}/upload`}>
    <span class="icon"><Icon name="upload" size={22} /></span>
    <span class="text">
      <span class="title">{t('takein.have_export')}</span>
      <span class="sub">{t('takein.have_export_body')}</span>
    </span>
    <Icon name="chevron-right" class="chev" />
  </a>
  <a class="option" href={`/take-in/source/${platform}/guide`}>
    <span class="icon"><Icon name="info" size={22} /></span>
    <span class="text">
      <span class="title">{t('takein.need_help')}</span>
      <span class="sub">{t('takein.need_help_body')}</span>
    </span>
    <Icon name="chevron-right" class="chev" />
  </a>

  {#snippet footer()}
    <a class="btn btn-lg" href="/take-in/sources">{t('common.back')}</a>
    <a class="link" href={continuePath(platform)}>{t('takein.skip_source', { platform: name })}</a>
  {/snippet}
</Step>

<style>
  .option {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 18px 18px 18px 16px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-lg);
    background: var(--surface);
    color: var(--text);
    text-decoration: none;
  }

  .option:hover {
    border-color: var(--text-muted);
  }

  .icon {
    width: 48px;
    height: 48px;
    flex: 0 0 48px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--radius-md);
    background: var(--surface-2);
    color: var(--text);
  }

  .text {
    flex: 1 1 auto;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .title {
    font-size: 16px;
    font-weight: 600;
  }

  .sub {
    color: var(--text-3);
    font-size: 14px;
  }

  .option :global(.chev) {
    color: var(--text-muted);
  }
</style>
