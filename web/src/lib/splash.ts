// Removes the splash from index.html once the app is ready.
//
// The animation always plays to the end, so the splash never flashes by. When
// loading takes longer, the finished logo stays until the first screen is ready.

const ANIMATION_MS = 1850
const MAX_WAIT_MS = 8000

let done = false
const started = performance.now()

export function hideSplash() {
  if (done) return
  done = true
  const el = document.getElementById('splash')
  if (!el) return
  const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const wait = reduced ? 0 : Math.max(0, ANIMATION_MS - (performance.now() - started))
  window.setTimeout(() => {
    el.classList.add('is-done')
    window.setTimeout(() => el.remove(), 400)
  }, wait)
}

// Never leave the splash up forever, even if something fails to load.
window.setTimeout(hideSplash, MAX_WAIT_MS)

/** Resolves when the given images have loaded and decoded, or after a timeout. */
export function preloadImages(urls: string[], timeoutMs = 4000): Promise<void> {
  if (urls.length === 0) return Promise.resolve()
  const loads = urls.map(
    (url) =>
      new Promise<void>((resolve) => {
        const img = new Image()
        img.decoding = 'async'
        img.src = url
        img.decode().then(resolve, resolve)
      }),
  )
  return Promise.race([
    Promise.all(loads).then(() => undefined),
    new Promise<void>((resolve) => window.setTimeout(resolve, timeoutMs)),
  ])
}
