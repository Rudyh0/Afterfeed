// A minimal router based on the History API. Links with a same-origin href
// are handled automatically, so pages use plain <a href="/…"> elements.

export type Params = Record<string, string>

class RouterState {
  path = $state(window.location.pathname)
  search = $state(window.location.search)

  get query(): URLSearchParams {
    return new URLSearchParams(this.search)
  }
}

export const router = new RouterState()

const scrollPositions = new Map<string, number>()
// How many entries this app has pushed, so back() knows whether there is an
// in-app page to return to.
let depth = 0

function update(replaceScroll = true) {
  router.path = window.location.pathname
  router.search = window.location.search
  if (replaceScroll) window.scrollTo(0, 0)
}

export function navigate(to: string, opts: { replace?: boolean; keepScroll?: boolean } = {}) {
  const url = new URL(to, window.location.origin)
  if (url.pathname + url.search === window.location.pathname + window.location.search) return
  scrollPositions.set(window.location.pathname + window.location.search, window.scrollY)
  if (opts.replace) {
    history.replaceState(null, '', url.pathname + url.search + url.hash)
  } else {
    history.pushState(null, '', url.pathname + url.search + url.hash)
    depth++
  }
  update(!opts.keepScroll)
}

/** Replaces the query string without adding a history entry. */
export function setQuery(params: Record<string, string | undefined>) {
  const q = new URLSearchParams(window.location.search)
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === '') q.delete(k)
    else q.set(k, v)
  }
  const s = q.toString()
  history.replaceState(null, '', window.location.pathname + (s ? `?${s}` : ''))
  router.search = window.location.search
}

/** Goes back to the previous page in the app, or to fallback when there is none. */
export function back(fallback = '/') {
  if (depth > 0) {
    history.back()
  } else {
    navigate(fallback, { replace: true })
  }
}

window.addEventListener('popstate', () => {
  depth = Math.max(0, depth - 1)
  update(false)
  const y = scrollPositions.get(window.location.pathname + window.location.search)
  requestAnimationFrame(() => window.scrollTo(0, y ?? 0))
})

document.addEventListener('click', (e) => {
  if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return
  const a = (e.target as Element | null)?.closest?.('a')
  if (!a) return
  const href = a.getAttribute('href')
  if (!href || !href.startsWith('/') || href.startsWith('//') || href.startsWith('/api/')) return
  if (a.target && a.target !== '_self') return
  if (a.hasAttribute('download')) return
  e.preventDefault()
  navigate(href)
})

/**
 * Matches a path against a pattern such as /years/:year/:month. Parameters
 * ending in ? may be left out at the end: /messages/:platform?/:id?.
 */
export function match(pattern: string, path: string): Params | null {
  const p = pattern.split('/').filter(Boolean)
  const s = path.split('/').filter(Boolean)
  const required = p.filter((x) => !x.endsWith('?')).length
  if (s.length < required || s.length > p.length) return null
  const params: Params = {}
  for (let i = 0; i < s.length; i++) {
    if (p[i].startsWith(':')) {
      params[p[i].slice(1).replace(/\?$/, '')] = decodeURIComponent(s[i])
    } else if (p[i] !== s[i]) {
      return null
    }
  }
  return params
}
