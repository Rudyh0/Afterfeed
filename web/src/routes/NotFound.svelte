<script lang="ts">
  import { onMount } from 'svelte'
  import { t } from '../lib/i18n/index.svelte'
  import { hideSplash } from '../lib/splash'

  onMount(hideSplash)
</script>

<div class="not-found">
  <p>{t('error.not_found')}</p>
  <a class="btn" href="/">{t('error.go_home')}</a>
</div>

<style>
  .not-found {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
    max-width: 520px;
    margin: 80px auto;
    font-size: 17px;
  }
</style>
