<script lang="ts">
  import { t } from '../lib/i18n/index.svelte'
  import Icon from './Icon.svelte'

  interface Props {
    count: number
    onedit: () => void
    ondownload: () => void
    onvault: () => void
    onclear: () => void
  }
  let { count, onedit, ondownload, onvault, onclear }: Props = $props()
</script>

<div class="bar" role="toolbar" aria-label={t('month.selection')}>
  <button class="icon-btn" type="button" aria-label={t('month.clear_selection')} onclick={onclear}><Icon name="close" size={20} /></button>
  <span class="count tabular">{t('count.selected', { count })}</span>
  <div class="actions">
    <button class="btn" type="button" disabled={count === 0} onclick={onedit}><Icon name="pencil" size={16} />{t('menu.edit_metadata')}</button>
    <button class="btn" type="button" disabled={count === 0} onclick={ondownload}><Icon name="download" size={16} />{t('menu.download')}</button>
    <button class="btn" type="button" disabled={count === 0} onclick={onvault}><Icon name="lock" size={16} />{t('menu.move_to_vault')}</button>
  </div>
</div>

<style>
  .bar {
    position: sticky;
    top: 12px;
    z-index: 10;
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 8px 12px;
    margin-bottom: 24px;
    padding: 8px 8px 8px 8px;
    border: 1px solid var(--border);
    border-radius: var(--radius-lg);
    background: var(--surface);
    box-shadow: var(--shadow-md);
  }

  .count {
    font-weight: 600;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-left: auto;
  }
</style>
