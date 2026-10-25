// The sources chosen in the Take-in and the order they are handled in. Kept
// in sessionStorage so a reload continues where you were.

export const platforms = ['facebook', 'instagram', 'x', 'snapchat'] as const
export type Platform = (typeof platforms)[number]

export function isPlatform(v: string): v is Platform {
  return (platforms as readonly string[]).includes(v)
}

function load(): Platform[] {
  try {
    const raw = sessionStorage.getItem('af_takein_queue')
    const list = raw ? (JSON.parse(raw) as string[]) : []
    return list.filter(isPlatform)
  } catch {
    return []
  }
}

export const takeIn = $state<{ queue: Platform[] }>({ queue: load() })

export function setQueue(q: Platform[]) {
  takeIn.queue = q
  try {
    sessionStorage.setItem('af_takein_queue', JSON.stringify(q))
  } catch {
    // Private mode; the queue only lives in memory.
  }
}

/** The platform after `current` in the queue, if any. */
export function nextAfter(current: string): Platform | undefined {
  const i = takeIn.queue.indexOf(current as Platform)
  return i >= 0 ? takeIn.queue[i + 1] : takeIn.queue[0]
}

/** The path to continue to after `current` is handled or skipped. */
export function continuePath(current: string): string {
  const next = nextAfter(current)
  return next ? `/take-in/source/${next}` : '/take-in/done'
}
