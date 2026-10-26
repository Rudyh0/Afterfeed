<script lang="ts">
  import { untrack } from 'svelte'
  import { patch } from '../lib/api'
  import { errorMessage } from '../lib/errors'
  import { formatFullDate, t } from '../lib/i18n/index.svelte'
  import { dateOnly, placeLabel, type Place } from '../lib/memories'
  import PlacePicker, { type PlaceValue } from './PlacePicker.svelte'

  export interface EditableMemory {
    id: string
    date: string
    time: string
    tz?: string
    text?: string
    place?: Place
    server_tz: string
    original?: { date: string; time: string; tz?: string; text: string; place?: Place }
  }

  interface Props {
    memory: EditableMemory
    onsaved: (m: EditableMemory) => void
    oncancel: () => void
  }
  let { memory, onsaved, oncancel }: Props = $props()

  // The form starts from the memory as it was when the editor opened.
  const start = untrack(() => memory)
  let date = $state(start.date)
  let time = $state(start.time)
  let tz = $state(start.tz ?? '')
  let text = $state(start.text ?? '')
  let place = $state<PlaceValue | null>(toValue(start.place))
  let busy = $state(false)
  let error = $state('')

  function toValue(p?: Place): PlaceValue | null {
    if (!p) return null
    return { name: p.name ?? '', lat: p.lat, lon: p.lon }
  }

  const zones = (() => {
    try {
      return (Intl as unknown as { supportedValuesOf(k: string): string[] }).supportedValuesOf('timeZone')
    } catch {
      return ['UTC', 'Europe/Amsterdam', 'Europe/London', 'Europe/Paris', 'Europe/Berlin', 'Europe/Madrid', 'America/New_York', 'America/Los_Angeles', 'Asia/Tokyo']
    }
  })()

  const o = $derived(memory.original)
  const timeChanged = $derived(!!o && (o.date !== memory.date || o.time !== memory.time || (o.tz ?? '') !== (memory.tz ?? '')))
  const textChanged = $derived(!!o && o.text !== (memory.text ?? ''))
  const placeChanged = $derived(!!o && (placeLabel(o.place) !== placeLabel(memory.place) || o.place?.lat !== memory.place?.lat || o.place?.lon !== memory.place?.lon))

  function apply(m: EditableMemory) {
    date = m.date
    time = m.time
    tz = m.tz ?? ''
    text = m.text ?? ''
    place = toValue(m.place)
    onsaved(m)
  }

  async function save(e: SubmitEvent) {
    e.preventDefault()
    error = ''
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date)) {
      error = t('editor.invalid_date')
      return
    }
    if (!/^\d{2}:\d{2}$/.test(time)) {
      error = t('editor.invalid_time')
      return
    }
    busy = true
    try {
      const m = await patch<EditableMemory>(`/memories/${memory.id}`, {
        date,
        time,
        tz,
        text,
        place: place && (place.name.trim() || place.lat !== undefined) ? place : null,
      })
      onsaved(m)
      oncancel()
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }

  async function restore(field: 'taken_at' | 'text' | 'place') {
    busy = true
    error = ''
    try {
      apply(await patch<EditableMemory>(`/memories/${memory.id}`, { restore: [field] }))
    } catch (err) {
      error = errorMessage(err)
    } finally {
      busy = false
    }
  }
</script>

<form class="editor" onsubmit={save}>
  <h2>{t('editor.title')}</h2>

  <fieldset>
    <div class="legend-row">
      <legend>{t('editor.date')} · {t('editor.time')}</legend>
      {#if timeChanged}<span class="pill pill-accent">{t('editor.changed')}</span>{/if}
    </div>
    <div class="two">
      <label class="field"><span class="visually-hidden">{t('editor.date')}</span>
        <input class="input" type="date" bind:value={date} required />
      </label>
      <label class="field"><span class="visually-hidden">{t('editor.time')}</span>
        <input class="input" type="time" bind:value={time} required />
      </label>
    </div>
    <label class="field">
      <span class="sub-label">{t('editor.timezone')}</span>
      <select class="input" bind:value={tz}>
        <option value="">{t('editor.server_tz', { tz: memory.server_tz })}</option>
        {#each zones as z (z)}<option value={z}>{z.replaceAll('_', ' ')}</option>{/each}
      </select>
    </label>
    {#if timeChanged && o}
      <p class="original">
        {t('editor.original', { value: `${formatFullDate(dateOnly(o.date), 'UTC')} ${o.time}` })}
        <button class="link" type="button" disabled={busy} onclick={() => restore('taken_at')}>{t('editor.restore')}</button>
      </p>
    {/if}
  </fieldset>

  <fieldset>
    <div class="legend-row">
      <legend>{t('editor.location')}</legend>
      {#if placeChanged}<span class="pill pill-accent">{t('editor.changed')}</span>{/if}
    </div>
    <PlacePicker value={place} onchange={(v) => (place = v)} id={`place-${memory.id}`} />
    {#if placeChanged && o}
      <p class="original">
        {t('editor.original', { value: placeLabel(o.place) || t('editor.no_location') })}
        <button class="link" type="button" disabled={busy} onclick={() => restore('place')}>{t('editor.restore')}</button>
      </p>
    {/if}
  </fieldset>

  <fieldset>
    <div class="legend-row">
      <legend><label for={`text-${memory.id}`}>{t('editor.description')}</label></legend>
      {#if textChanged}<span class="pill pill-accent">{t('editor.changed')}</span>{/if}
    </div>
    <textarea id={`text-${memory.id}`} class="input textarea" bind:value={text} rows="5"></textarea>
    {#if textChanged && o}
      <p class="original">
        {t('editor.original', { value: o.text ? (o.text.length > 80 ? o.text.slice(0, 80) + '…' : o.text) : t('editor.no_description') })}
        <button class="link" type="button" disabled={busy} onclick={() => restore('text')}>{t('editor.restore')}</button>
      </p>
    {/if}
  </fieldset>

  {#if error}<p class="error-text" role="alert">{error}</p>{/if}

  <div class="actions">
    <button class="btn btn-lg" type="button" onclick={oncancel}>{t('common.cancel')}</button>
    <button class="btn btn-primary btn-lg" type="submit" disabled={busy}>{t('common.save')}</button>
  </div>
</form>

<style>
  .editor {
    display: flex;
    flex-direction: column;
    gap: 22px;
  }

  h2 {
    font-size: 22px;
    font-weight: 600;
    letter-spacing: -0.02em;
  }

  fieldset {
    display: flex;
    flex-direction: column;
    gap: 10px;
    margin: 0;
    padding: 0;
    border: 0;
  }

  .legend-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  legend {
    padding: 0;
    font-size: 14px;
    font-weight: 600;
  }

  .two {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  .sub-label {
    display: block;
    margin-bottom: 4px;
    color: var(--text-3);
    font-size: 13px;
  }

  .textarea {
    height: auto;
    min-height: 120px;
    padding: 12px 14px;
    line-height: 1.5;
    resize: vertical;
  }

  .original {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 4px 10px;
    color: var(--text-3);
    font-size: 13px;
  }

  .original .link {
    font-size: 13px;
  }

  .actions {
    display: flex;
    justify-content: space-between;
    gap: 12px;
    margin-top: 4px;
  }
</style>
