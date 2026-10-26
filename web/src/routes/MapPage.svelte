<script lang="ts">
  import 'maplibre-gl/dist/maplibre-gl.css'
  import * as maplibregl from 'maplibre-gl'
  import type { Map as MLMap, Marker } from 'maplibre-gl'
  import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
  import { onDestroy, onMount } from 'svelte'
  import Icon from '../components/Icon.svelte'
  import { get, mediaUrl } from '../lib/api'
  import { countryByName, findCountries } from '../lib/countries'
  import { errorMessage } from '../lib/errors'
  import { countryName, formatShortDate, formatNumber, t } from '../lib/i18n/index.svelte'
  import { library } from '../lib/library.svelte'
  import { dateOnly, placeLabel, sourceLabel, type Card } from '../lib/memories'
  import { navigate, router, setQuery, type Params } from '../lib/router.svelte'
  import { hideSplash } from '../lib/splash'

  let {}: { params?: Params } = $props()

  interface Cluster {
    lat: number
    lon: number
    count: number
    memory_id: string
    media_id?: string
    kind?: string
  }
  interface PlaceGroup {
    city: string
    country_code: string
    count: number
    first: string
    last: string
    lat: number
    lon: number
  }
  interface InView {
    total: number
    memories: Card[]
    places?: PlaceGroup[]
    years?: number[]
    countries?: string[]
  }
  interface GeoPlace {
    name: string
    country_code: string
    lat: number
    lon: number
  }

  let container = $state<HTMLDivElement>()
  let map: MLMap | null = null
  let markers: Marker[] = []
  let styleUrl = $state<string | null>(null)
  let error = $state('')
  let view = $state<InView | null>(null)
  let query = $state('')
  let suggestions = $state<{ label: string; detail: string; go: () => void }[]>([])
  let timer: ReturnType<typeof setTimeout> | undefined
  let searchTimer: ReturnType<typeof setTimeout> | undefined
  let empty = $state(false)

  const year = $derived(router.query.get('year') ?? '')
  const platform = $derived(router.query.get('platform') ?? '')
  const years = $derived.by(() => {
    const out: number[] = []
    for (const s of library.sources) for (let y = s.first_year; y <= s.last_year && s.first_year; y++) out.push(y)
    return [...new Set(out)].sort((a, b) => b - a)
  })

  function filterParams(): string {
    const p = new URLSearchParams()
    if (year) p.set('year', year)
    if (platform) p.set('platform', platform)
    return p.toString()
  }

  onMount(async () => {
    try {
      const s = await get<{ map_style: string }>('/settings')
      styleUrl = s.map_style || ''
      if (!styleUrl || !container) return
      // The worker is a separate file; the bundler puts it next to the app.
      maplibregl.setWorkerUrl(workerUrl)
      const m = new maplibregl.Map({
        container,
        style: styleUrl,
        center: [10, 30],
        zoom: 1.5,
        attributionControl: { compact: true },
        dragRotate: false,
        pitchWithRotate: false,
        locale: { 'NavigationControl.ZoomIn': t('map.zoom_in'), 'NavigationControl.ZoomOut': t('map.zoom_out') },
      })
      m.touchZoomRotate.disableRotation()
      m.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'bottom-right')
      m.on('moveend', schedule)
      // Show the memories even when the map tiles cannot be loaded.
      let started = false
      const once = () => {
        if (!started) {
          started = true
          start()
        }
      }
      m.on('load', once)
      setTimeout(once, 2500)
      map = m
    } catch (err) {
      error = errorMessage(err)
    } finally {
      hideSplash()
    }
  })

  onDestroy(() => {
    clearTimeout(timer)
    map?.remove()
  })

  async function start() {
    const memory = router.query.get('memory')
    try {
      const res = await get<{ bbox: number[] | null }>(memory ? `/map/bounds?memory=${memory}` : `/map/bounds?${filterParams()}`)
      if (!res.bbox) {
        empty = true
        refresh()
        return
      }
      fit(res.bbox, memory ? 14 : 12)
    } catch {
      refresh()
    }
  }

  function fit(b: number[], maxZoom: number) {
    if (!map) return
    if (b[0] === b[2] && b[1] === b[3]) {
      map.jumpTo({ center: [b[0], b[1]], zoom: maxZoom })
    } else {
      map.fitBounds([[b[0], b[1]], [b[2], b[3]]], { padding: 60, maxZoom, duration: 0 })
    }
    refresh()
  }

  function schedule() {
    clearTimeout(timer)
    timer = setTimeout(refresh, 250)
  }

  // Reload when the filters in the address change.
  $effect(() => {
    void year
    void platform
    if (map) refresh()
  })

  async function refresh() {
    if (!map) return
    const b = map.getBounds()
    const bbox = [b.getWest(), b.getSouth(), b.getEast(), b.getNorth()].map((n) => n.toFixed(5)).join(',')
    const extra = filterParams()
    try {
      const [clusters, inView] = await Promise.all([
        get<{ clusters: Cluster[] }>(`/map/clusters?bbox=${bbox}&zoom=${map.getZoom().toFixed(2)}${extra ? `&${extra}` : ''}`),
        get<InView>(`/map/memories?bbox=${bbox}&limit=20${extra ? `&${extra}` : ''}`),
      ])
      draw(clusters.clusters)
      view = inView
    } catch (err) {
      error = errorMessage(err)
    }
  }

  function draw(list: Cluster[]) {
    markers.forEach((m) => m.remove())
    markers = []
    if (!map) return
    const highlight = router.query.get('memory')
    for (const c of list) {
      const el = document.createElement('button')
      el.type = 'button'
      el.className = 'af-marker' + (c.memory_id === highlight && c.count === 1 ? ' current' : '')
      el.setAttribute('aria-label', c.count > 1 ? t('count.memories', { count: c.count }) : t('memory.about'))
      if (c.media_id) {
        const img = document.createElement('img')
        img.src = mediaUrl(c.media_id, 'thumb')
        img.alt = ''
        img.loading = 'lazy'
        el.appendChild(img)
      } else {
        el.classList.add('no-media')
      }
      if (c.count > 1) {
        const badge = document.createElement('span')
        badge.className = 'af-count'
        badge.textContent = formatNumber(c.count)
        el.appendChild(badge)
      }
      el.addEventListener('click', (e) => {
        e.stopPropagation()
        if (c.count > 1 && map && map.getZoom() < 16) {
          map.flyTo({ center: [c.lon, c.lat], zoom: Math.min(18, map.getZoom() + 2.5) })
        } else {
          navigate(`/memory/${c.memory_id}?from=${encodeURIComponent('/map' + window.location.search)}`)
        }
      })
      markers.push(new maplibregl.Marker({ element: el }).setLngLat([c.lon, c.lat]).addTo(map))
    }
  }

  function onSearch() {
    clearTimeout(searchTimer)
    const q = query.trim()
    if (q.length < 2) {
      suggestions = []
      return
    }
    searchTimer = setTimeout(async () => {
      const out: typeof suggestions = []
      for (const code of findCountries(q)) {
        out.push({ label: countryName(code), detail: '', go: () => goCountry(code) })
      }
      try {
        const res = await get<{ places: GeoPlace[] }>(`/geo/search?q=${encodeURIComponent(q)}`)
        for (const p of res.places.slice(0, 6)) {
          out.push({ label: p.name, detail: countryName(p.country_code), go: () => goPlace(p) })
        }
      } catch {
        // Only countries then.
      }
      suggestions = out
    }, 200)
  }

  async function goCountry(code: string) {
    suggestions = []
    query = countryName(code)
    const res = await get<{ bbox: number[] | null }>(`/map/bounds?country=${code}`)
    if (res.bbox) fit(res.bbox, 12)
  }

  function goPlace(p: GeoPlace) {
    suggestions = []
    query = p.name
    map?.flyTo({ center: [p.lon, p.lat], zoom: 11 })
  }

  function submitSearch(e: SubmitEvent) {
    e.preventDefault()
    const code = countryByName(query)
    if (code) goCountry(code)
    else suggestions[0]?.go()
  }

  function inViewTitle(v: InView): string {
    const names = (v.countries ?? []).map((c) => countryName(c))
    return names.length > 0 && names.length <= 2 ? names.join(', ') : t('map.in_view')
  }

  function yearsText(ys: number[]): string {
    if (ys.length === 0) return ''
    if (ys.length <= 3) return ys.join(', ')
    return `${ys[0]}–${ys[ys.length - 1]}`
  }

  function monthYear(d: string): string {
    return new Intl.DateTimeFormat(document.documentElement.lang, { month: 'long', year: 'numeric', timeZone: 'UTC' }).format(dateOnly(d))
  }
</script>

<h1 class="visually-hidden">{t('map.title')}</h1>

{#if styleUrl === ''}
  <div class="off">
    <p>{t('map.not_configured')}</p>
    <a class="btn" href="/settings/map">{t('nav.settings')}</a>
  </div>
{:else}
  <div class="map-page">
    <div class="map-wrap">
      <div class="map" bind:this={container}></div>

      <div class="controls">
        <form class="search" onsubmit={submitSearch} role="search">
          <Icon name="search" size={18} />
          <input bind:value={query} oninput={onSearch} placeholder={t('map.search_place')} aria-label={t('map.search_place')} autocomplete="off" />
          {#if suggestions.length > 0}
            <ul class="suggestions">
              {#each suggestions as s, i (i)}
                <li><button type="button" onclick={s.go}><span>{s.label}</span>{#if s.detail}<span class="muted">{s.detail}</span>{/if}</button></li>
              {/each}
            </ul>
          {/if}
        </form>
        <div class="filters">
          <select class="input select" aria-label={t('years.all_years')} value={year}
                  onchange={(e) => setQuery({ year: (e.currentTarget as HTMLSelectElement).value || undefined })}>
            <option value="">{t('years.all_years')}</option>
            {#each years as y (y)}<option value={String(y)}>{y}</option>{/each}
          </select>
          {#if library.sources.length > 1}
            <select class="input select" aria-label={t('search.all_sources')} value={platform}
                    onchange={(e) => setQuery({ platform: (e.currentTarget as HTMLSelectElement).value || undefined })}>
              <option value="">{t('search.all_sources')}</option>
              {#each library.sources as s (s.platform)}<option value={s.platform}>{sourceLabel(s.platform)}</option>{/each}
            </select>
          {/if}
        </div>
      </div>

      {#if empty}
        <div class="empty card">
          <h2>{t('map.empty_title')}</h2>
          <p>{t('map.empty_body')}</p>
        </div>
      {/if}
    </div>

    <aside class="panel" aria-label={t('map.in_view')}>
      {#if error}<p class="error-text" role="alert">{error}</p>{/if}
      {#if view}
        <div class="panel-head">
          <p class="eyebrow">{t('map.in_view')}</p>
          <h2>{inViewTitle(view)}</h2>
          {#if view.total > 0}
            <p class="muted">{t('map.summary', { memories: t('count.memories', { count: view.total }), years: yearsText(view.years ?? []) })}</p>
          {/if}
        </div>
        {#if view.places && view.places.length > 1}
          <ul class="places">
            {#each view.places as p (p.city + p.country_code)}
              <li>
                <button type="button" onclick={() => map?.flyTo({ center: [p.lon, p.lat], zoom: Math.max(map.getZoom(), 11) })}>
                  <span class="place-name">{p.city || countryName(p.country_code) || t('common.unknown')}{#if p.city && p.country_code}<span class="muted">, {countryName(p.country_code)}</span>{/if}</span>
                  <span class="muted tabular">{formatNumber(p.count)} · {p.first.slice(0, 7) === p.last.slice(0, 7) ? monthYear(p.first) : `${p.first.slice(0, 4)}–${p.last.slice(0, 4)}`}</span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
        <ul class="list">
          {#each view.memories as m (m.id)}
            <li>
              <a href={`/memory/${m.id}?from=${encodeURIComponent('/map' + window.location.search)}`}>
                {#if m.media[0]}
                  <img src={mediaUrl(m.media[0].id, 'thumb')} alt="" loading="lazy" />
                {:else}
                  <span class="noimg"><Icon name="pin" size={18} /></span>
                {/if}
                <span class="info">
                  <span class="date tabular">{formatShortDate(dateOnly(m.date), 'UTC')}</span>
                  {#if m.text}<span class="text">{m.text}</span>{/if}
                  <span class="muted">{placeLabel(m.place)}</span>
                </span>
              </a>
            </li>
          {/each}
        </ul>
      {/if}
    </aside>
  </div>
{/if}

<style>
  .map-page {
    display: flex;
    height: 100vh;
    min-height: 0;
  }

  .map-wrap {
    position: relative;
    flex: 1 1 auto;
    min-width: 0;
  }

  .map {
    position: absolute;
    inset: 0;
    background: var(--surface-2);
  }

  :global(:root[data-theme='dark']) .map :global(.maplibregl-canvas) {
    filter: invert(1) hue-rotate(180deg) brightness(0.92) contrast(0.88);
  }

  @media (prefers-color-scheme: dark) {
    :global(:root[data-theme='system']) .map :global(.maplibregl-canvas) {
      filter: invert(1) hue-rotate(180deg) brightness(0.92) contrast(0.88);
    }
  }

  .controls {
    position: absolute;
    top: 16px;
    left: 16px;
    right: 16px;
    z-index: 2;
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    pointer-events: none;
  }

  .controls > * {
    pointer-events: auto;
  }

  .search {
    position: relative;
    flex: 0 1 360px;
    display: flex;
    align-items: center;
    gap: 8px;
    height: 44px;
    padding: 0 14px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface);
    box-shadow: var(--shadow-md);
    color: var(--text-3);
  }

  .search input {
    flex: 1 1 auto;
    min-width: 0;
    border: 0;
    outline: none;
    background: transparent;
    color: var(--text);
    font: inherit;
    font-size: 15px;
  }

  .suggestions {
    position: absolute;
    top: calc(100% + 6px);
    left: 0;
    right: 0;
    margin: 0;
    padding: 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface);
    box-shadow: var(--shadow-lg);
    list-style: none;
  }

  .suggestions button {
    width: 100%;
    display: flex;
    align-items: baseline;
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

  .suggestions button:hover {
    background: var(--surface-2);
  }

  .filters {
    display: flex;
    gap: 8px;
  }

  .select {
    width: auto;
    height: 44px;
    box-shadow: var(--shadow-md);
  }

  .empty {
    position: absolute;
    left: 50%;
    top: 50%;
    z-index: 2;
    max-width: 360px;
    padding: 20px 22px;
    transform: translate(-50%, -50%);
  }

  .empty h2 {
    margin-bottom: 6px;
    font-size: 17px;
  }

  .empty p {
    color: var(--text-3);
    font-size: 14px;
  }

  .panel {
    flex: 0 0 360px;
    display: flex;
    flex-direction: column;
    gap: 16px;
    overflow-y: auto;
    padding: 24px 20px;
    border-left: 1px solid var(--border);
    background: var(--surface);
  }

  .panel-head h2 {
    font-size: 22px;
    font-weight: 600;
    letter-spacing: -0.02em;
  }

  .muted {
    color: var(--text-muted);
    font-size: 13px;
  }

  .places,
  .list {
    display: flex;
    flex-direction: column;
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .places {
    padding-bottom: 12px;
    border-bottom: 1px solid var(--divider);
  }

  .places button {
    width: 100%;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 8px 10px;
    border: 0;
    border-radius: var(--radius);
    background: transparent;
    color: var(--text);
    font: inherit;
    text-align: left;
    cursor: pointer;
  }

  .places button:hover {
    background: var(--surface-2);
  }

  .place-name {
    font-size: 14px;
    font-weight: 600;
  }

  .list a {
    display: flex;
    gap: 12px;
    padding: 8px;
    border-radius: var(--radius-md);
    color: var(--text);
    text-decoration: none;
  }

  .list a:hover {
    background: var(--surface-2);
  }

  .list img,
  .noimg {
    flex: 0 0 56px;
    width: 56px;
    height: 56px;
    border-radius: var(--radius);
    object-fit: cover;
    background: var(--surface-2);
  }

  .noimg {
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
  }

  .info {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .date {
    font-size: 13px;
    font-weight: 600;
  }

  .text {
    overflow: hidden;
    font-size: 14px;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .off {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 16px;
    padding: 48px;
    color: var(--text-3);
  }

  :global(.af-marker) {
    position: relative;
    width: 52px;
    height: 52px;
    padding: 0;
    overflow: visible;
    border: 3px solid #fff;
    border-radius: 50%;
    background: #d9dce2;
    box-shadow: 0 2px 8px rgba(17, 18, 20, 0.35);
    cursor: pointer;
  }

  :global(.af-marker img) {
    width: 100%;
    height: 100%;
    display: block;
    border-radius: 50%;
    object-fit: cover;
  }

  :global(.af-marker.no-media) {
    width: 22px;
    height: 22px;
    background: var(--ink);
  }

  :global(.af-marker.current) {
    border-color: var(--accent);
    transform: scale(1.15);
  }

  :global(.af-count) {
    position: absolute;
    top: -8px;
    right: -10px;
    min-width: 24px;
    height: 24px;
    padding: 0 6px;
    border-radius: 999px;
    background: var(--ink);
    color: var(--on-ink);
    font-size: 12px;
    font-weight: 600;
    line-height: 24px;
    text-align: center;
  }

  @media (max-width: 900px) {
    .map-page {
      flex-direction: column;
      height: auto;
    }

    .map-wrap {
      height: 62vh;
      flex: 0 0 auto;
    }

    .panel {
      flex: 1 1 auto;
      border-left: 0;
      padding-bottom: 96px;
    }

    .search {
      flex: 1 1 100%;
    }
  }
</style>
