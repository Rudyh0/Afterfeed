<script lang="ts">
  import { post } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { formatShortDate, formatTime, t } from '../lib/i18n/index.svelte'
  import type { Card } from '../lib/memories'
  import Dialog from './Dialog.svelte'
  import PlacePicker, { type PlaceValue } from './PlacePicker.svelte'

  interface Props {
    open: boolean
    cards: Card[]
    onclose: () => void
    ondone: () => void
  }
  let { open, cards, onclose, ondone }: Props = $props()

  let placeMode = $state<'leave' | 'set'>('leave')
  let place = $state<PlaceValue | null>(null)
  let overwrite = $state(false)
  let timeMode = $state<'leave' | 'shift' | 'set'>('leave')
  let hours = $state(0)
  let minutes = $state(0)
  let date = $state('')
  let time = $state('12:00')
  let busy = $state(false)
  let error = $state('')

  const withPlace = $derived(cards.filter((c) => c.place?.lat !== undefined).length)
  const first = $derived([...cards].sort((a, b) => a.taken_at.localeCompare(b.taken_at))[0])
  const preview = $derived.by(() => {
    if (!first || timeMode !== 'shift') return ''
    const before = new Date(first.taken_at)
    const after = new Date(before.getTime() + (hours * 60 + minutes) * 60000)
    const fmt = (d: Date) => `${formatShortDate(d)}, ${formatTime(d)}`
    return t('bulk.preview_first', { before: fmt(before), after: fmt(after) })
  })
  const canApply = $derived(
    !busy &&
      ((placeMode === 'set' && !!place && (place.name.trim() !== '' || place.lat !== undefined)) ||
        (timeMode === 'shift' && hours * 60 + minutes !== 0) ||
        (timeMode === 'set' && !!date && !!time)),
  )

  async function apply() {
    busy = true
    error = ''
    const body: Record<string, unknown> = { ids: cards.map((c) => c.id) }
    if (placeMode === 'set' && place) body.place = { ...place, overwrite }
    if (timeMode === 'shift') body.shift_minutes = hours * 60 + minutes
    if (timeMode === 'set') Object.assign(body, { date, time, tz: Intl.DateTimeFormat().resolvedOptions().timeZone })
    try {
      await post('/memories/bulk', body)
      ondone()
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }
</script>

<Dialog {open} {onclose} title={t('bulk.title', { count: cards.length })}>
  <fieldset>
    <legend>{t('editor.location')}</legend>
    <div class="segmented" role="group">
      <button type="button" aria-pressed={placeMode === 'leave'} onclick={() => (placeMode = 'leave')}>{t('bulk.leave')}</button>
      <button type="button" aria-pressed={placeMode === 'set'} onclick={() => (placeMode = 'set')}>{t('bulk.set')}</button>
    </div>
    {#if placeMode === 'set'}
      <PlacePicker value={place} onchange={(v) => (place = v)} id="bulk-place" />
      {#if withPlace > 0}
        <label class="check"><input type="checkbox" bind:checked={overwrite} />{t('bulk.overwrite', { count: withPlace })}</label>
      {/if}
    {/if}
  </fieldset>

  <fieldset>
    <legend>{t('bulk.date_time')}</legend>
    <div class="segmented" role="group">
      <button type="button" aria-pressed={timeMode === 'leave'} onclick={() => (timeMode = 'leave')}>{t('bulk.leave')}</button>
      <button type="button" aria-pressed={timeMode === 'shift'} onclick={() => (timeMode = 'shift')}>{t('bulk.shift')}</button>
      <button type="button" aria-pressed={timeMode === 'set'} onclick={() => (timeMode = 'set')}>{t('bulk.set')}</button>
    </div>
    {#if timeMode === 'shift'}
      <div class="shift">
        <label><input class="input num" type="number" min="-999" max="999" bind:value={hours} /> {t('bulk.hours')}</label>
        <label><input class="input num" type="number" min="-59" max="59" bind:value={minutes} /> {t('bulk.minutes')}</label>
      </div>
      {#if preview}<p class="muted">{preview}</p>{/if}
    {:else if timeMode === 'set'}
      <div class="two">
        <input class="input" type="date" bind:value={date} aria-label={t('editor.date')} />
        <input class="input" type="time" bind:value={time} aria-label={t('editor.time')} />
      </div>
    {/if}
  </fieldset>

  {#if error}<p class="error-text" role="alert">{error}</p>{/if}

  {#snippet footer()}
    <button class="btn" type="button" onclick={onclose}>{t('common.cancel')}</button>
    <button class="btn btn-primary" type="button" disabled={!canApply} onclick={apply}>{t('bulk.apply', { count: cards.length })}</button>
  {/snippet}
</Dialog>

<style>
  fieldset {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin: 0;
    padding: 0;
    border: 0;
  }

  legend {
    margin-bottom: 8px;
    padding: 0;
    font-size: 14px;
    font-weight: 600;
  }

  .check {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 14px;
  }

  .check input {
    width: 18px;
    height: 18px;
    accent-color: var(--ink);
  }

  .shift {
    display: flex;
    flex-wrap: wrap;
    gap: 12px;
  }

  .shift label {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14px;
  }

  .num {
    width: 96px;
  }

  .two {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .muted {
    color: var(--text-3);
    font-size: 13px;
  }
</style>
