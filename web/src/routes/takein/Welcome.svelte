<script lang="ts">
  import { locale, localeNames, locales, setLocale, t } from '../../lib/i18n/index.svelte'
  import Step from './Step.svelte'
</script>

<Step title={t('takein.welcome_title')} body={t('takein.welcome_body')}>
  <div class="field">
    <span class="field-label" id="lang-label">{t('takein.language_title')}</span>
    <div class="languages" role="group" aria-labelledby="lang-label">
      {#each locales as l (l)}
        <button type="button" class="lang" aria-pressed={locale() === l} onclick={() => setLocale(l)}>
          {localeNames[l]}
        </button>
      {/each}
    </div>
  </div>

  {#snippet footer()}
    <span></span>
    <a class="btn btn-primary btn-lg" href="/take-in/account">{t('takein.start')}</a>
  {/snippet}
</Step>

<style>
  .languages {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
    gap: 8px;
  }

  .lang {
    height: 48px;
    padding: 0 16px;
    border: 1px solid var(--border-strong);
    border-radius: var(--radius-md);
    background: var(--surface);
    color: var(--text);
    font-size: 15px;
    font-weight: 500;
    text-align: left;
  }

  .lang[aria-pressed='true'] {
    border-color: var(--ink);
    box-shadow: inset 0 0 0 1px var(--ink);
    font-weight: 600;
  }
</style>
