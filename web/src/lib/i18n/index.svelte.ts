// Translations and locale-aware formatting.
//
// Text lives in one JSON file per language. Keys are dotted paths. A key with
// plural forms has variants with a suffix from Intl.PluralRules, for example
// "count.memories_one" and "count.memories_other"; t() picks the right one
// when a numeric `count` is passed.

import de from './de.json'
import en from './en.json'
import es from './es.json'
import fr from './fr.json'
import nl from './nl.json'

export const locales = ['en', 'nl', 'fr', 'de', 'es'] as const
export type Locale = (typeof locales)[number]

export const localeNames: Record<Locale, string> = {
  en: 'English',
  nl: 'Nederlands',
  fr: 'Français',
  de: 'Deutsch',
  es: 'Español',
}

type Dict = Record<string, string>

function flatten(obj: Record<string, unknown>, prefix = '', out: Dict = {}): Dict {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object') flatten(v as Record<string, unknown>, key, out)
    else out[key] = String(v)
  }
  return out
}

const dicts: Record<Locale, Dict> = {
  en: flatten(en),
  nl: flatten(nl),
  fr: flatten(fr),
  de: flatten(de),
  es: flatten(es),
}

export function isLocale(v: unknown): v is Locale {
  return typeof v === 'string' && (locales as readonly string[]).includes(v)
}

function readCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((c) => c.startsWith(`${name}=`))
    ?.split('=')[1]
}

function detect(): Locale {
  const fromCookie = readCookie('af_lang')
  if (isLocale(fromCookie)) return fromCookie
  for (const lang of navigator.languages ?? [navigator.language]) {
    const base = lang.toLowerCase().split('-')[0]
    if (isLocale(base)) return base
  }
  return 'en'
}

const state = $state<{ locale: Locale }>({ locale: detect() })
document.documentElement.lang = state.locale

export function locale(): Locale {
  return state.locale
}

export function setLocale(l: Locale) {
  state.locale = l
  document.documentElement.lang = l
  document.cookie = `af_lang=${l}; Path=/; Max-Age=31536000; SameSite=Lax`
}

const pluralCache = new Map<string, Intl.PluralRules>()
function plural(l: Locale, n: number): string {
  let rules = pluralCache.get(l)
  if (!rules) {
    rules = new Intl.PluralRules(l)
    pluralCache.set(l, rules)
  }
  return rules.select(n)
}

export type Vars = Record<string, string | number>

/** Translates a key. Unknown keys fall back to English, then to the key itself. */
export function t(key: string, vars?: Vars): string {
  const l = state.locale
  const d = dicts[l]
  let text: string | undefined
  if (vars && typeof vars.count === 'number') {
    const form = plural(l, vars.count)
    text = d[`${key}_${form}`] ?? d[`${key}_other`] ?? dicts.en[`${key}_${plural('en', vars.count)}`] ?? dicts.en[`${key}_other`]
  }
  text ??= d[key] ?? dicts.en[key]
  if (text === undefined) {
    if (import.meta.env.DEV) console.warn(`missing translation: ${key}`)
    return key
  }
  if (!vars) return text
  return text.replace(/\{(\w+)\}/g, (m, name) => {
    const v = vars[name]
    if (v === undefined) return m
    return typeof v === 'number' ? formatNumber(v) : v
  })
}

/** Returns every translation of a key, used to recognise words in any language. */
export function allTranslations(key: string): string[] {
  const out = new Set<string>()
  for (const l of locales) {
    const v = dicts[l][key]
    if (v) v.split('|').forEach((w) => out.add(w.trim().toLowerCase()))
  }
  return [...out]
}

/** Returns the translation of a key in the current language, split on |. */
export function words(key: string): string[] {
  const v = dicts[state.locale][key] ?? dicts.en[key] ?? ''
  return v.split('|').map((w) => w.trim().toLowerCase()).filter(Boolean)
}

// --- formatting ---

const fmtCache = new Map<string, Intl.DateTimeFormat>()
function dtf(opts: Intl.DateTimeFormatOptions, timeZone?: string): Intl.DateTimeFormat {
  const key = `${state.locale}|${timeZone ?? ''}|${JSON.stringify(opts)}`
  let f = fmtCache.get(key)
  if (!f) {
    f = new Intl.DateTimeFormat(state.locale, { ...opts, timeZone })
    fmtCache.set(key, f)
  }
  return f
}

function asDate(d: Date | string): Date {
  return typeof d === 'string' ? new Date(d) : d
}

export function formatNumber(n: number): string {
  return new Intl.NumberFormat(state.locale).format(n)
}

/** "25 October", "25 oktober", "25. Oktober" */
export function formatDayMonth(d: Date | string, timeZone?: string): string {
  return dtf({ day: 'numeric', month: 'long' }, timeZone).format(asDate(d))
}

/** "Friday 25 October 2019" */
export function formatFullDate(d: Date | string, timeZone?: string): string {
  const s = dtf({ weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }, timeZone).format(asDate(d))
  return s.charAt(0).toUpperCase() + s.slice(1)
}

/** "25 Oct 2019" */
export function formatShortDate(d: Date | string, timeZone?: string): string {
  return dtf({ day: 'numeric', month: 'short', year: 'numeric' }, timeZone).format(asDate(d))
}

/** "25 Oct" */
export function formatShortDayMonth(d: Date | string, timeZone?: string): string {
  return dtf({ day: 'numeric', month: 'short' }, timeZone).format(asDate(d))
}

export function formatTime(d: Date | string, timeZone?: string): string {
  return dtf({ hour: '2-digit', minute: '2-digit' }, timeZone).format(asDate(d))
}

export function formatWeekday(d: Date | string, timeZone?: string): string {
  return dtf({ weekday: 'long' }, timeZone).format(asDate(d))
}

/** Month name for a 1-based month number, capitalised: "October", "Oktober". */
export function monthName(month: number, style: 'long' | 'short' = 'long'): string {
  const s = dtf({ month: style }, 'UTC').format(new Date(Date.UTC(2000, month - 1, 15)))
  return s.charAt(0).toUpperCase() + s.slice(1)
}

/** All month names in a locale, lower case, used to recognise months in search. */
export function monthNames(l: Locale): string[][] {
  const long = new Intl.DateTimeFormat(l, { month: 'long', timeZone: 'UTC' })
  const short = new Intl.DateTimeFormat(l, { month: 'short', timeZone: 'UTC' })
  return Array.from({ length: 12 }, (_, i) => {
    const d = new Date(Date.UTC(2000, i, 15))
    return [long.format(d), short.format(d)].map((s) => s.toLowerCase().replace(/\.$/, ''))
  })
}

export function formatBytes(bytes: number): string {
  const units = ['B', 'kB', 'MB', 'GB', 'TB']
  let i = 0
  let v = bytes
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000
    i++
  }
  const n = new Intl.NumberFormat(state.locale, { maximumFractionDigits: v < 10 && i > 0 ? 1 : 0 }).format(v)
  return `${n} ${units[i]}`
}

export function formatDuration(seconds: number): string {
  const s = Math.round(seconds)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const r = s % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return h > 0 ? `${h}:${pad(m)}:${pad(r)}` : `${m}:${pad(r)}`
}

/** Localised country name for an ISO 3166 code. */
export function countryName(code: string, l: Locale = state.locale): string {
  if (!code) return ''
  try {
    return new Intl.DisplayNames([l], { type: 'region' }).of(code.toUpperCase()) ?? code
  } catch {
    return code
  }
}
