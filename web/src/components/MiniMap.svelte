<script lang="ts">
  // A small map for choosing a location. Loaded only when needed, because
  // the map library is large.
  import 'maplibre-gl/dist/maplibre-gl.css'
  import * as maplibregl from 'maplibre-gl'
  import workerUrl from 'maplibre-gl/dist/maplibre-gl-worker.mjs?url'
  import { onDestroy, onMount } from 'svelte'
  import { t } from '../lib/i18n/index.svelte'

  interface Props {
    styleUrl: string
    lat?: number
    lon?: number
    onpick: (lat: number, lon: number) => void
  }
  let { styleUrl, lat, lon, onpick }: Props = $props()

  let el = $state<HTMLDivElement>()
  let map: maplibregl.Map | null = null
  let marker: maplibregl.Marker | null = null

  onMount(() => {
    if (!el) return
    maplibregl.setWorkerUrl(workerUrl)
    map = new maplibregl.Map({
      container: el,
      style: styleUrl,
      center: lat !== undefined && lon !== undefined ? [lon, lat] : [10, 40],
      zoom: lat !== undefined ? 11 : 2,
      attributionControl: { compact: true },
      dragRotate: false,
      locale: { 'NavigationControl.ZoomIn': t('map.zoom_in'), 'NavigationControl.ZoomOut': t('map.zoom_out') },
    })
    map.addControl(new maplibregl.NavigationControl({ showCompass: false }), 'bottom-right')
    map.on('click', (e) => {
      place(e.lngLat.lat, e.lngLat.lng)
      onpick(e.lngLat.lat, e.lngLat.lng)
    })
    if (lat !== undefined && lon !== undefined) place(lat, lon)
  })

  onDestroy(() => map?.remove())

  function place(la: number, lo: number) {
    if (!map) return
    if (!marker) {
      marker = new maplibregl.Marker({ draggable: true, color: '#17181c' }).setLngLat([lo, la]).addTo(map)
      marker.on('dragend', () => {
        const p = marker!.getLngLat()
        onpick(p.lat, p.lng)
      })
    } else {
      marker.setLngLat([lo, la])
    }
  }

  // Follow changes from outside, such as choosing a place in the search.
  $effect(() => {
    if (map && lat !== undefined && lon !== undefined) {
      place(lat, lon)
      map.flyTo({ center: [lon, lat], zoom: Math.max(map.getZoom(), 10) })
    }
  })
</script>

<div class="mini" bind:this={el}></div>

<style>
  .mini {
    height: 200px;
    overflow: hidden;
    border: 1px solid var(--border);
    border-radius: var(--radius-md);
    background: var(--surface-2);
  }

  :global(:root[data-theme='dark']) .mini :global(.maplibregl-canvas) {
    filter: invert(1) hue-rotate(180deg) brightness(0.92) contrast(0.88);
  }

  @media (prefers-color-scheme: dark) {
    :global(:root[data-theme='system']) .mini :global(.maplibregl-canvas) {
      filter: invert(1) hue-rotate(180deg) brightness(0.92) contrast(0.88);
    }
  }
</style>
