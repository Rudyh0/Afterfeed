<script lang="ts">
  import { onDestroy, onMount } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import { readPhotoInfo } from '../lib/exif'
  import { formatNumber, t } from '../lib/i18n/index.svelte'
  import { loadLibrary } from '../lib/library.svelte'
  import { navigate, type Params } from '../lib/router.svelte'
  import { hideSplash } from '../lib/splash'

  let {}: { params?: Params } = $props()

  interface Item {
    key: string
    file: File
    url: string
    video: boolean
  }

  type LocationMode = 'manual' | 'current' | 'photo'

  const now = new Date()
  const pad = (n: number) => String(n).padStart(2, '0')

  let items = $state<Item[]>([])
  let text = $state('')
  let date = $state(`${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`)
  let time = $state(`${pad(now.getHours())}:${pad(now.getMinutes())}`)
  let dateTouched = $state(false)
  let dateFromPhoto = $state(false)
  let mode = $state<LocationMode>('manual')
  let placeName = $state('')
  let coords = $state<{ lat: number; lon: number; accuracy?: number } | null>(null)
  let photoCoords = $state<{ lat: number; lon: number } | null>(null)
  let locating = $state(false)
  let locationError = $state('')
  let saving = $state(false)
  let progress = $state(0)
  let error = $state('')
  const touch = typeof matchMedia !== 'undefined' && matchMedia('(pointer: coarse)').matches
  const secure = typeof window !== 'undefined' && window.isSecureContext

  const canSave = $derived((items.length > 0 || text.trim() !== '') && !saving)

  onMount(hideSplash)
  onDestroy(() => items.forEach((i) => URL.revokeObjectURL(i.url)))

  async function add(files: FileList | null) {
    if (!files) return
    for (const file of Array.from(files)) {
      if (!file.type.startsWith('image/') && !file.type.startsWith('video/') && !/\.(heic|heif|jpe?g|png|mov|mp4)$/i.test(file.name)) continue
      items.push({ key: crypto.randomUUID(), file, url: URL.createObjectURL(file), video: file.type.startsWith('video/') })
      if (items.length === 1 || (!dateFromPhoto && !photoCoords)) {
        const info = await readPhotoInfo(file)
        if (info.date && !dateTouched) {
          date = info.date
          time = info.time ?? time
          dateFromPhoto = true
        }
        if (info.lat !== undefined && info.lon !== undefined && !photoCoords) {
          photoCoords = { lat: info.lat, lon: info.lon }
          if (mode === 'manual' && !placeName) mode = 'photo'
        }
      }
    }
  }

  function remove(item: Item) {
    URL.revokeObjectURL(item.url)
    items = items.filter((i) => i.key !== item.key)
    if (items.length === 0) {
      dateFromPhoto = false
      photoCoords = null
      if (mode === 'photo') mode = 'manual'
    }
  }

  function useCurrent() {
    mode = 'current'
    locationError = ''
    if (!secure || !navigator.geolocation) {
      locationError = t('new_memory.location_https_only')
      return
    }
    locating = true
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        coords = { lat: pos.coords.latitude, lon: pos.coords.longitude, accuracy: pos.coords.accuracy }
        locating = false
      },
      () => {
        locationError = t('new_memory.location_denied')
        locating = false
      },
      { enableHighAccuracy: true, timeout: 20000, maximumAge: 60000 },
    )
  }

  function coordText(c: { lat: number; lon: number }): string {
    return `${c.lat.toFixed(5)}, ${c.lon.toFixed(5)}`
  }

  function place() {
    const name = placeName.trim()
    if (mode === 'current' && coords) return { name, lat: coords.lat, lon: coords.lon }
    if (mode === 'photo' && photoCoords) return { name, lat: photoCoords.lat, lon: photoCoords.lon }
    if (mode === 'manual' && name) return { name }
    return null
  }

  function save(e: SubmitEvent) {
    e.preventDefault()
    if (!canSave) return
    error = ''
    saving = true
    progress = 0
    const form = new FormData()
    form.append(
      'data',
      JSON.stringify({
        text: text.trim(),
        date,
        time,
        tz: Intl.DateTimeFormat().resolvedOptions().timeZone,
        date_auto: !dateTouched && !dateFromPhoto,
        place: place(),
      }),
    )
    for (const i of items) form.append('file', i.file, i.file.name)

    const xhr = new XMLHttpRequest()
    xhr.open('POST', '/api/memories')
    xhr.setRequestHeader('X-Afterfeed', '1')
    xhr.upload.onprogress = (ev) => {
      if (ev.lengthComputable) progress = ev.loaded / ev.total
    }
    xhr.onload = () => {
      saving = false
      if (xhr.status === 200) {
        const card = JSON.parse(xhr.responseText) as { id: string }
        loadLibrary()
        navigate(`/memory/${card.id}?from=/`, { replace: true })
      } else {
        let code = ''
        try {
          code = JSON.parse(xhr.responseText)?.error?.code ?? ''
        } catch {
          // Not JSON.
        }
        error = code === 'new_memory.empty' ? t('new_memory.empty') : code === 'new_memory.unsupported' ? t('new_memory.unsupported') : t('error.internal')
      }
    }
    xhr.onerror = () => {
      saving = false
      error = t('error.network')
    }
    xhr.send(form)
  }
</script>

<form class="page" onsubmit={save}>
  <h1 class="page-title">{t('new_memory.title')}</h1>

  <section class="card block">
    {#if items.length > 0}
      <ul class="previews">
        {#each items as item (item.key)}
          <li>
            {#if item.video}
              <!-- svelte-ignore a11y_media_has_caption -->
              <video src={item.url} muted playsinline preload="metadata"></video>
              <span class="play" aria-hidden="true"><Icon name="play" size={14} /></span>
            {:else}
              <img src={item.url} alt="" />
            {/if}
            <button type="button" class="remove" aria-label={t('new_memory.remove_media', { name: item.file.name })} onclick={() => remove(item)}>
              <Icon name="close" size={16} />
            </button>
          </li>
        {/each}
      </ul>
    {/if}
    <div class="pickers">
      <label class="btn">
        <Icon name="image" size={18} />{t('new_memory.add_media')}
        <input class="visually-hidden" type="file" accept="image/*,video/*" multiple onchange={(e) => add((e.currentTarget as HTMLInputElement).files)} />
      </label>
      {#if touch}
        <label class="btn">
          <Icon name="plus" size={18} />{t('new_memory.take_photo')}
          <input class="visually-hidden" type="file" accept="image/*" capture="environment" onchange={(e) => add((e.currentTarget as HTMLInputElement).files)} />
        </label>
      {/if}
    </div>
  </section>

  <div class="field">
    <label for="nm-text">{t('new_memory.text')}</label>
    <textarea id="nm-text" class="input textarea" bind:value={text} rows="4"></textarea>
  </div>

  <div class="two">
    <div class="field">
      <label for="nm-date">{t('new_memory.date')}</label>
      <input id="nm-date" class="input" type="date" bind:value={date} oninput={() => ((dateTouched = true), (dateFromPhoto = false))} required />
    </div>
    <div class="field">
      <label for="nm-time">{t('new_memory.time')}</label>
      <input id="nm-time" class="input" type="time" bind:value={time} oninput={() => ((dateTouched = true), (dateFromPhoto = false))} required />
    </div>
  </div>
  {#if dateFromPhoto}<p class="hint">{t('new_memory.date_from_photo')}</p>{/if}

  <fieldset class="field location">
    <legend>{t('new_memory.location')}</legend>
    <div class="segmented wrap" role="group">
      <button type="button" aria-pressed={mode === 'manual'} onclick={() => (mode = 'manual')}>{t('new_memory.enter_location')}</button>
      <button type="button" aria-pressed={mode === 'current'} onclick={useCurrent}><Icon name="pin" size={16} />{t('new_memory.use_current_location')}</button>
      {#if photoCoords}
        <button type="button" aria-pressed={mode === 'photo'} onclick={() => (mode = 'photo')}><Icon name="image" size={16} />{t('new_memory.location_from_photo')}</button>
      {/if}
    </div>

    {#if mode === 'current'}
      {#if locating}
        <p class="hint">{t('new_memory.locating')}</p>
      {:else if locationError}
        <p class="error-text" role="alert">{locationError}</p>
      {:else if coords}
        <p class="coords"><Icon name="pin" size={16} /><span class="tabular">{coordText(coords)}</span>
          {#if coords.accuracy}<span class="hint">· {t('new_memory.accuracy', { meters: formatNumber(Math.round(coords.accuracy)) })}</span>{/if}</p>
      {/if}
    {:else if mode === 'photo' && photoCoords}
      <p class="coords"><Icon name="pin" size={16} /><span class="tabular">{coordText(photoCoords)}</span></p>
    {/if}

    <label class="visually-hidden" for="nm-place">{t('new_memory.place_name')}</label>
    <input id="nm-place" class="input" bind:value={placeName}
           placeholder={mode === 'manual' ? t('new_memory.place_name') : t('new_memory.place_name_optional')} />
  </fieldset>

  {#if error}<p class="error-text" role="alert">{error}</p>{/if}
  {#if saving && items.length > 0}
    <div class="bar" role="progressbar" aria-valuemin="0" aria-valuemax="100" aria-valuenow={Math.round(progress * 100)}>
      <span style:width={`${Math.round(progress * 100)}%`}></span>
    </div>
  {/if}

  <div class="actions">
    <button class="btn btn-lg" type="button" onclick={() => history.back()}>{t('common.cancel')}</button>
    <button class="btn btn-primary btn-lg" type="submit" disabled={!canSave}>{saving ? t('new_memory.saving') : t('new_memory.save')}</button>
  </div>
</form>

<style>
  .page {
    max-width: 640px;
    margin: 0 auto;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .block {
    display: flex;
    flex-direction: column;
    gap: 16px;
    padding: 16px;
  }

  .previews {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
    gap: 8px;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .previews li {
    position: relative;
    aspect-ratio: 1;
    overflow: hidden;
    border-radius: var(--radius-md);
    background: var(--surface-2);
  }

  .previews img,
  .previews video {
    width: 100%;
    height: 100%;
    display: block;
    object-fit: cover;
  }

  .play {
    position: absolute;
    left: 8px;
    bottom: 8px;
    display: flex;
    padding: 5px;
    border-radius: 50%;
    background: rgba(17, 18, 20, 0.6);
    color: #fff;
  }

  .remove {
    position: absolute;
    top: 6px;
    right: 6px;
    width: 32px;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 0;
    border-radius: 50%;
    background: rgba(17, 18, 20, 0.6);
    color: #fff;
    cursor: pointer;
  }

  .pickers {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
  }

  .pickers label {
    cursor: pointer;
  }

  .pickers label:focus-within {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
  }

  .textarea {
    height: auto;
    min-height: 120px;
    padding: 12px 14px;
    resize: vertical;
    line-height: 1.5;
  }

  .two {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 12px;
  }

  .hint {
    color: var(--text-muted);
    font-size: 13px;
  }

  .two + .hint {
    margin-top: -12px;
  }

  .location {
    display: flex;
    flex-direction: column;
    gap: 12px;
    margin: 0;
    padding: 0;
    border: 0;
  }

  legend {
    margin-bottom: 8px;
    padding: 0;
    font-size: 14px;
    font-weight: 500;
  }

  .segmented.wrap {
    flex-wrap: wrap;
  }

  .segmented button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .coords {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 6px;
    font-size: 14px;
  }

  .bar {
    height: 4px;
    overflow: hidden;
    border-radius: 2px;
    background: var(--surface-3);
  }

  .bar span {
    display: block;
    height: 100%;
    background: var(--ink);
  }

  .actions {
    display: flex;
    justify-content: space-between;
    gap: 12px;
  }
</style>
