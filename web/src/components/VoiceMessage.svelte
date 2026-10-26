<script lang="ts" module>
  // Only one voice message plays at a time.
  let playing: HTMLAudioElement | null = null
</script>

<script lang="ts">
  import { onDestroy } from 'svelte'
  import { get } from '../lib/api'
  import { formatDuration, t } from '../lib/i18n/index.svelte'
  import { fileUrl, type MessageFile } from '../lib/messages'
  import Icon from './Icon.svelte'

  let { file }: { file: MessageFile } = $props()

  const bars = 36
  let audio = $state<HTMLAudioElement>()
  let points = $state<number[]>([])
  let paused = $state(true)
  let current = $state(0)
  let total = $state(0)
  let loaded = false

  $effect(() => {
    total = file.duration ?? 0
  })

  // The waveform is loaded when the message comes into view.
  function observe(node: HTMLElement) {
    const io = new IntersectionObserver((entries) => {
      if (entries.some((e) => e.isIntersecting) && !loaded) {
        loaded = true
        io.disconnect()
        get<{ points: number[] }>(`/message-files/${file.id}/waveform`)
          .then((r) => (points = r.points))
          .catch(() => {})
      }
    })
    io.observe(node)
    return { destroy: () => io.disconnect() }
  }

  // Resample the curve to the number of bars, with a minimum height so
  // silence still shows as a line.
  const heights = $derived.by(() => {
    if (points.length === 0) return Array.from({ length: bars }, () => 0.18)
    return Array.from({ length: bars }, (_, i) => {
      const a = Math.floor((i * points.length) / bars)
      const b = Math.max(a + 1, Math.floor(((i + 1) * points.length) / bars))
      const slice = points.slice(a, b)
      return Math.max(0.12, slice.reduce((x, y) => Math.max(x, y), 0))
    })
  })

  const progress = $derived(total > 0 ? current / total : 0)

  function toggle() {
    if (!audio) return
    if (audio.paused) {
      if (playing && playing !== audio) playing.pause()
      playing = audio
      audio.play().catch(() => {})
    } else {
      audio.pause()
    }
  }

  function seek(e: MouseEvent) {
    if (!audio || !total) return
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    audio.currentTime = Math.min(total, Math.max(0, ((e.clientX - rect.left) / rect.width) * total))
    if (audio.paused) toggle()
  }

  function onKey(e: KeyboardEvent) {
    if (!audio || !total) return
    if (e.key === 'ArrowRight') audio.currentTime = Math.min(total, audio.currentTime + 5)
    else if (e.key === 'ArrowLeft') audio.currentTime = Math.max(0, audio.currentTime - 5)
    else return
    e.preventDefault()
  }

  onDestroy(() => {
    if (playing === audio) playing = null
  })
</script>

<div class="voice" use:observe>
  <button class="play" type="button" onclick={toggle}
          aria-label={paused ? t('messages.play_voice', { duration: formatDuration(total) }) : t('messages.pause')}>
    <Icon name={paused ? 'play' : 'pause'} size={20} />
  </button>
  <div class="wave" role="slider" tabindex="0" aria-label={t('messages.voice')}
       aria-valuemin={0} aria-valuemax={Math.round(total)} aria-valuenow={Math.round(current)}
       aria-valuetext={`${formatDuration(current)} / ${formatDuration(total)}`}
       onclick={seek} onkeydown={onKey}>
    {#each heights as h, i (i)}
      <span class:played={(i + 0.5) / bars <= progress} style:height={`${Math.round(h * 28)}px`}></span>
    {/each}
  </div>
  <span class="time tabular">{formatDuration(paused && current === 0 ? total : current)}</span>
  <audio bind:this={audio} src={fileUrl(file.id, 'original')} preload="none"
         onplay={() => (paused = false)} onpause={() => (paused = true)}
         onended={() => { paused = true; current = 0 }}
         ontimeupdate={() => (current = audio?.currentTime ?? 0)}
         onloadedmetadata={() => { if (audio && isFinite(audio.duration)) total = audio.duration }}></audio>
</div>

<style>
  .voice {
    width: 300px;
    max-width: 100%;
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 6px 14px 6px 6px;
  }

  .play {
    flex: 0 0 44px;
    width: 44px;
    height: 44px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 0;
    border-radius: 50%;
    background: var(--ink);
    color: var(--on-ink);
    cursor: pointer;
  }

  .wave {
    flex: 1 1 auto;
    min-width: 0;
    height: 32px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 2px;
    cursor: pointer;
    border-radius: 4px;
  }

  .wave span {
    flex: 0 1 3px;
    min-width: 2px;
    border-radius: 2px;
    background: var(--text-3);
    opacity: 0.45;
  }

  .wave span.played {
    background: var(--ink);
    opacity: 1;
  }

  .time {
    flex: 0 0 auto;
    min-width: 34px;
    color: var(--text-3);
    font-size: 13px;
    text-align: right;
  }
</style>
