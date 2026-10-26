<script lang="ts">
  import { onMount, untrack, type Component } from 'svelte'
  import { get } from '../lib/api'
  import { countryName, t } from '../lib/i18n/index.svelte'
  import Icon from './Icon.svelte'

  export interface PlaceValue {
    name: string
    lat?: number
    lon?: number
  }

  interface Props {
    value: PlaceValue | null
    onchange: (v: PlaceValue | null) => void
    id?: string
  }
  let { value, onchange, id = 'place' }: Props = $props()

  interface GeoPlace {
    name: string
    country_code: string
    lat: number
    lon: number
  }

  let name = $state(untrack(() => value?.name ?? ''))
  let hits = $state<GeoPlace[]>([])
  let timer: ReturnType<typeof setTimeout> | undefined
  let styleUrl = $state('')
  let MiniMap = $state<Component<{ styleUrl: string; lat?: number; lon?: number; onpick: (lat: number, lon: number) => void }> | null>(null)

  onMount(async () => {
    try {
      const s = await get<{ map_style: string }>('/settings')
      styleUrl = s.map_style
      if (styleUrl) MiniMap = (await import('./MiniMap.svelte')).default
    } catch {
      // No map; the name and search still work.
    }
  })

  function onInput() {
    onchange({ name, lat: value?.lat, lon: value?.lon })
    clearTimeout(timer)
    const q = name.trim()
    if (q.length < 2) {
      hits = []
      return
    }
    timer = setTimeout(async () => {
      try {
        hits = (await get<{ places: GeoPlace[] }>(`/geo/search?q=${encodeURIComponent(q)}`)).places.slice(0, 5)
      } catch {
        hits = []
      }
    }, 200)
  }

  function choose(p: GeoPlace) {
    name = p.name
    hits = []
    onchange({ name: p.name, lat: p.lat, lon: p.lon })
  }

  function pick(lat: number, lon: number) {
    onchange({ name, lat, lon })
  }

  function clear() {
    name = ''
    hits = []
    onchange(null)
  }
</script>

<div class="picker">
  <div class="row">
    <div class="name">
      <label class="visually-hidden" for={id}>{t('editor.search_place')}</label>
      <input {id} class="input" bind:value={name} oninput={onInput} placeholder={t('editor.search_place')} autocomplete="off" />
      {#if hits.length > 0}
        <ul class="hits">
          {#each hits as h (h.name + h.lat)}
            <li><button type="button" onclick={() => choose(h)}><span>{h.name}</span><span class="muted">{countryName(h.country_code)}</span></button></li>
          {/each}
        </ul>
      {/if}
    </div>
    {#if value}
      <button class="btn" type="button" onclick={clear}><Icon name="close" size={16} />{t('editor.clear_location')}</button>
    {/if}
  </div>
  {#if value?.lat !== undefined && value?.lon !== undefined}
    <p class="coords tabular"><Icon name="pin" size={14} />{value.lat.toFixed(5)}, {value.lon.toFixed(5)}</p>
  {/if}
  {#if MiniMap && styleUrl}
    <MiniMap {styleUrl} lat={value?.lat} lon={value?.lon} onpick={pick} />
    <p class="muted">{t('editor.pick_on_map')}</p>
  {/if}
</div>

<style>
  .picker {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .row {
    display: flex;
    gap: 8px;
  }

  .name {
    position: relative;
    flex: 1 1 auto;
  }

  .hits {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    z-index: 20;
    margin: 0;
    padding: 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
    list-style: none;
  }

  .hits button {
    width: 100%;
    display: flex;
    justify-content: space-between;
    gap: 12px;
    min-height: 40px;
    padding: 0 10px;
    border: 0;
    border-radius: var(--radius);
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 14px;
    text-align: left;
    cursor: pointer;
  }

  .hits button:hover {
    background: var(--surface-2);
  }

  .coords {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--text-3);
    font-size: 13px;
  }

  .muted {
    color: var(--text-muted);
    font-size: 13px;
  }
</style>
