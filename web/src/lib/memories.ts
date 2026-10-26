// Types and helpers for memories as the API returns them.

import { countryName, formatDayMonth, t } from './i18n/index.svelte'

export interface Place {
  name?: string
  city?: string
  region?: string
  country_code?: string
  lat?: number
  lon?: number
}

export interface CardMedia {
  id: string
  kind: 'photo' | 'video' | string
  mime: string
  width: number
  height: number
  duration?: number
  overlay?: boolean
  text?: string
}

export interface Card {
  id: string
  platform: string
  kind: string
  taken_at: string
  /** Local date where it happened, YYYY-MM-DD. */
  date: string
  /** Local time where it happened, HH:MM. */
  time: string
  year: number
  tz?: string
  text?: string
  link?: string
  album?: string
  place?: Place
  media: CardMedia[]
  media_count: number
  photo_count: number
  video_count: number
  vault?: boolean
  edited?: boolean
}

/** A readable label for a place: its name, or the city and country. */
export function placeLabel(p?: Place): string {
  if (!p) return ''
  if (p.name) return p.name
  const parts = [p.city, p.country_code ? countryName(p.country_code) : ''].filter(Boolean)
  return parts.join(', ')
}

/** The second line under a place name: city and country, if different from the name. */
export function placeDetail(p?: Place): string {
  if (!p?.name) return ''
  const parts = [p.city, p.region, p.country_code ? countryName(p.country_code) : '']
    .filter((x): x is string => !!x && !p.name!.includes(x))
  return [...new Set(parts)].join(', ')
}

/** Today's month and day in the browser's time zone, as MM-DD. */
export function todayMd(): string {
  const d = new Date()
  return `${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

/** Adds days to a MM-DD date, in a leap year so 29 February exists. */
export function addDays(md: string, n: number): string {
  const [m, d] = md.split('-').map(Number)
  const date = new Date(Date.UTC(2000, m - 1, d + n))
  return `${String(date.getUTCMonth() + 1).padStart(2, '0')}-${String(date.getUTCDate()).padStart(2, '0')}`
}

export function isMd(v: string | undefined): v is string {
  if (!v || !/^\d{2}-\d{2}$/.test(v)) return false
  const [m, d] = v.split('-').map(Number)
  const date = new Date(Date.UTC(2000, m - 1, d))
  return date.getUTCMonth() === m - 1 && date.getUTCDate() === d
}

/** "26 October" for a MM-DD date. */
export function formatMd(md: string): string {
  return formatDayMonth(`2000-${md}T12:00:00Z`, 'UTC')
}

/** A local date (YYYY-MM-DD) as a Date at noon UTC, for formatting with timeZone UTC. */
export function dateOnly(d: string): Date {
  return new Date(`${d}T12:00:00Z`)
}

/** "7 years ago", or "This year". */
export function yearsAgo(year: number): string {
  const n = new Date().getFullYear() - year
  return n <= 0 ? t('time.this_year') : t('time.years_ago', { count: n })
}

/** Video length as 0:42 or 1:05:12. */
export function duration(seconds?: number): string {
  const s = Math.max(0, Math.round(seconds ?? 0))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = String(s % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${sec}` : `${m}:${sec}`
}

/** The label of the source of a memory. */
export function sourceLabel(platform: string): string {
  return platform === 'afterfeed' ? t('kind.memory') : t(`platform.${platform}`)
}

/** A short description of the media, such as "4 photos" or "Video". */
export function mediaSummary(c: Card): string {
  if (c.media_count === 0) return ''
  if (c.media_count === 1) return c.video_count === 1 ? t('kind.video') : ''
  if (c.video_count === 0) return t('count.photos', { count: c.photo_count })
  if (c.photo_count === 0) return t('count.videos', { count: c.video_count })
  return `${t('count.photos', { count: c.photo_count })} · ${t('count.videos', { count: c.video_count })}`
}

/** The domain of a link, for showing it compactly. */
export function linkHost(url: string): string {
  try {
    return new URL(url).hostname.replace(/^www\./, '')
  } catch {
    return url
  }
}
