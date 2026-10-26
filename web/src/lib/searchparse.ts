// Recognises periods in a search: years, seasons and months, in the user's
// language and in English. What is left is searched as text.

import { formatShortDate, locale, monthNames, t, words } from './i18n/index.svelte'

export interface Period {
  label: string
  from?: string
  to?: string
  months?: number[]
  /** The words of the query that make up the period. */
  tokens: string[]
}

export interface ParsedQuery {
  text: string
  period?: Period
}

const SEASONS: Record<string, number[]> = {
  spring: [3, 4, 5],
  summer: [6, 7, 8],
  autumn: [9, 10, 11],
  winter: [12, 1, 2],
}

const isYear = (s: string) => /^(19|20)\d{2}$/.test(s)
const pad = (n: number) => String(n).padStart(2, '0')
const lastDay = (y: number, m: number) => new Date(Date.UTC(y, m, 0)).getUTCDate()
const cap = (s: string) => s.charAt(0).toUpperCase() + s.slice(1)

function seasonWords(): Map<string, string> {
  const map = new Map<string, string>()
  for (const s of Object.keys(SEASONS)) {
    for (const w of words(`search.words.${s}`)) map.set(w, s)
    map.set(s, s)
  }
  map.set('fall', 'autumn')
  return map
}

function monthWords(): Map<string, number> {
  const map = new Map<string, number>()
  for (const l of [locale(), 'en'] as const) {
    monthNames(l).forEach((names, i) => names.forEach((n) => n.length >= 3 && map.set(n, i + 1)))
  }
  return map
}

export function parseQuery(raw: string): ParsedQuery {
  const tokens = raw.trim().split(/\s+/).filter(Boolean)
  const lower = tokens.map((x) => x.toLowerCase().replace(/[.,]$/, ''))
  const seasons = seasonWords()
  const months = monthWords()

  for (let i = 0; i < lower.length; i++) {
    const w = lower[i]
    const next = lower[i + 1]
    const yearNext = next && isYear(next) ? Number(next) : undefined
    const season = seasons.get(w)
    const month = months.get(w)
    let period: Period | undefined
    let used = 1

    if (season) {
      const ms = SEASONS[season]
      const label = cap(t(`season.${season}`)) + (yearNext ? ` ${yearNext}` : '')
      if (yearNext) {
        used = 2
        const y = yearNext
        const from = season === 'winter' ? `${y}-12-01` : `${y}-${pad(ms[0])}-01`
        const endY = season === 'winter' ? y + 1 : y
        const endM = ms[ms.length - 1]
        const to = `${endY}-${pad(endM)}-${lastDay(endY, endM)}`
        period = { label, from, to, tokens: [] }
      } else {
        period = { label, months: ms, tokens: [] }
      }
    } else if (month) {
      if (yearNext) {
        used = 2
        period = {
          label: `${cap(tokens[i])} ${yearNext}`,
          from: `${yearNext}-${pad(month)}-01`,
          to: `${yearNext}-${pad(month)}-${lastDay(yearNext, month)}`,
          tokens: [],
        }
      } else {
        period = { label: cap(tokens[i]), months: [month], tokens: [] }
      }
    } else if (isYear(w)) {
      period = { label: w, from: `${w}-01-01`, to: `${w}-12-31`, tokens: [] }
    }

    if (period) {
      period.tokens = tokens.slice(i, i + used)
      const rest = [...tokens.slice(0, i), ...tokens.slice(i + used)]
      // Drop a preposition right before the period, such as "in 2019".
      const prepositions = new Set([...words('search.words.in'), 'in'])
      if (i > 0 && prepositions.has(lower[i - 1])) rest.splice(i - 1, 1)
      return { text: rest.join(' '), period }
    }
  }
  return { text: raw.trim() }
}

/** "Summer 2021: 1 Jun 2021 to 31 Aug 2021" */
export function describePeriod(p: Period): string {
  if (!p.from || !p.to) return p.label
  if (p.from.endsWith('-01-01') && p.to.endsWith('-12-31') && p.from.slice(0, 4) === p.to.slice(0, 4)) return p.label
  return t('search.period', {
    label: p.label,
    from: formatShortDate(`${p.from}T12:00:00Z`, 'UTC'),
    to: formatShortDate(`${p.to}T12:00:00Z`, 'UTC'),
  })
}
