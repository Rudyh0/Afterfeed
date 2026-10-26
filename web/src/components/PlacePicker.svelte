<script lang="ts">
  // Chooses a real place: typing shows matching places right away, and only
  // a place from the list, a point on the map or nothing can be chosen.
  import { onMount, tick, type Component } from 'svelte'
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
    /** Show the small map for picking a point. */
    map?: boolean
  }
  let { value, onchange, id = 'place', map = true }: Props = $props()

  interface GeoPlace {
    name: string
    country_code: string
    lat: number
    lon: number
  }

  let query = $state('')
  let editing = $state(false)
  let open = $state(false)
  let hits = $state<GeoPlace[]>([])
  let searched = $state('')
  let active = $state(0)
  let hint = $state('')
  let input = $state<HTMLInputElement>()
  let timer: ReturnType<typeof setTimeout> | undefined
  let styleUrl = $state('')
  let MiniMap = $state<Component<{ styleUrl: string; lat?: number; lon?: number; onpick: (lat: number, lon: number) => void }> | null>(null)

  // What the field shows when nobody is typing in it.
  const shown = $derived(editing ? query : (value?.name ?? ''))
  const listId = $derived(`${id}-list`)

  onMount(async () => {
    if (!map) return
    try {
      const s = await get<{ map_style: string }>('/settings')
      styleUrl = s.map_style
      if (styleUrl) MiniMap = (await import('./MiniMap.svelte')).default
    } catch {
      // No map; searching still works.
    }
  })

  function onInput(e: Event) {
    query = (e.currentTarget as HTMLInputElement).value
    editing = true
    hint = ''
    clearTimeout(timer)
    const q = query.trim()
    if (!q) {
      hits = []
      open = false
      return
    }
    timer = setTimeout(() => search(q), 120)
  }

  async function search(q: string) {
    try {
      const res = await get<{ places: GeoPlace[] }>(`/geo/search?q=${encodeURIComponent(q)}`)
      if (query.trim() !== q) return
      hits = res.places
      searched = q
      active = 0
      open = true
    } catch {
      hits = []
    }
  }

  function choose(p: GeoPlace) {
    clearTimeout(timer)
    editing = false
    open = false
    hits = []
    hint = ''
    onchange({ name: p.name, lat: p.lat, lon: p.lon })
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'ArrowDown' && hits.length > 0) {
      open = true
      active = (active + 1) % hits.length
    } else if (e.key === 'ArrowUp' && hits.length > 0) {
      open = true
      active = (active - 1 + hits.length) % hits.length
    } else if (e.key === 'Enter') {
      // Enter chooses the highlighted place instead of sending the form.
      if (editing) {
        e.preventDefault()
        if (open && hits[active]) choose(hits[active])
      }
    } else if (e.key === 'Escape' && (open || editing)) {
      e.preventDefault()
      e.stopPropagation()
      cancelEdit()
    } else {
      return
    }
  }

  // Leaving the field without choosing keeps the place that was there.
  function cancelEdit() {
    if (editing && query.trim() !== '') hint = t('editor.choose_from_list')
    editing = false
    open = false
    hits = []
  }

  function onBlur() {
    // A click on a suggestion lands after the blur.
    setTimeout(() => {
      if (document.activeElement !== input) cancelEdit()
    }, 150)
  }

  async function pick(lat: number, lon: number) {
    let name = ''
    try {
      const res = await get<{ place: GeoPlace | null }>(`/geo/reverse?lat=${lat}&lon=${lon}`)
      name = res.place?.name ?? ''
    } catch {
      // The point alone is enough.
    }
    hint = ''
    onchange({ name, lat, lon })
  }

  async function clear() {
    editing = false
    query = ''
    hits = []
    open = false
    hint = ''
    onchange(null)
    await tick()
    input?.focus()
  }
</script>

<div class="picker">
  <div class="row">
    <div class="name">
      <label class="visually-hidden" for={id}>{t('editor.search_place')}</label>
      <span class="search-icon" aria-hidden="true"><Icon name="search" size={18} /></span>
      <input {id} bind:this={input} class="input" value={shown} oninput={onInput} onkeydown={onKey} onblur={onBlur}
             onfocus={() => { if (hits.length > 0 && editing) open = true }}
             placeholder={t('editor.search_place')} autocomplete="off" spellcheck="false"
             role="combobox" aria-autocomplete="list" aria-expanded={open} aria-controls={listId}
             aria-activedescendant={open && hits[active] ? `${id}-opt-${active}` : undefined} />
      {#if open}
        <ul class="hits" id={listId} role="listbox">
          {#each hits as h, i (i)}
            <li id={`${id}-opt-${i}`} role="option" aria-selected={i === active}>
              <button type="button" class:active={i === active} tabindex="-1"
                      onmousedown={(e) => e.preventDefault()} onclick={() => choose(h)}>
                <Icon name="pin" size={16} />
                <span class="place">{h.name}</span>
                <span class="muted">{countryName(h.country_code)}</span>
              </button>
            </li>
          {:else}
            <li class="none" role="option" aria-selected="false" aria-disabled="true">{t('editor.no_places', { q: searched })}</li>
          {/each}
        </ul>
      {/if}
    </div>
    {#if value}
      <button class="btn" type="button" onclick={clear}><Icon name="close" size={16} />{t('editor.clear_location')}</button>
    {/if}
  </div>
  {#if hint}
    <p class="hint" role="status">{hint}</p>
  {:else if value?.lat !== undefined && value?.lon !== undefined}
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

  .search-icon {
    position: absolute;
    top: 13px;
    left: 12px;
    display: flex;
    color: var(--text-muted);
    pointer-events: none;
  }

  .name .input {
    padding-left: 38px;
  }

  .hits {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    right: 0;
    z-index: 20;
    max-height: 320px;
    overflow-y: auto;
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
    align-items: center;
    gap: 10px;
    min-height: 44px;
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

  .hits button :global(svg) {
    flex: 0 0 auto;
    color: var(--text-muted);
  }

  .hits button:hover,
  .hits button.active {
    background: var(--surface-2);
  }

  .place {
    flex: 1 1 auto;
    min-width: 0;
    overflow: hidden;
    white-space: nowrap;
    text-overflow: ellipsis;
  }

  .none {
    padding: 12px 10px;
    color: var(--text-muted);
    font-size: 14px;
  }

  .coords {
    display: flex;
    align-items: center;
    gap: 6px;
    color: var(--text-3);
    font-size: 13px;
  }

  .hint {
    color: var(--danger);
    font-size: 13px;
  }

  .muted {
    flex: 0 0 auto;
    color: var(--text-muted);
    font-size: 13px;
  }
</style>
