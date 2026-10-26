// Types and helpers for conversations and messages.
import { t } from './i18n/index.svelte'

export interface Conversation {
  id: string
  platform: string
  title: string
  is_group: boolean
  participants: string[]
  self?: string
  first_at: number
  last_at: number
  last_text: string
  message_count: number
  file_count: number
  vault?: boolean
}

export interface MessageFile {
  id: string
  kind: 'photo' | 'video' | 'audio' | 'document' | 'other' | string
  name?: string
  mime?: string
  size?: number
  width?: number
  height?: number
  duration?: number
}

export interface Reaction {
  actor?: string
  reaction: string
}

export interface Message {
  id: number
  sender: string
  self: boolean
  /** Milliseconds since 1970. */
  at: number
  kind: string
  text?: string
  link?: string
  files?: MessageFile[]
  reactions?: Reaction[]
}

export interface MessagePage {
  conversation: Conversation
  messages: Message[]
  more_before: boolean
  more_after: boolean
}

export interface PlatformCount {
  platform: string
  conversations: number
}

/** Messages from Facebook are what people know as Messenger. */
export function platformLabel(platform: string): string {
  return t(platform === 'facebook' ? 'platform.messenger' : `platform.${platform}`)
}

/** "2014–2023", or one year when the conversation fits in it. */
export function yearRange(c: Pick<Conversation, 'first_at' | 'last_at'>): string {
  if (!c.first_at) return ''
  const a = new Date(c.first_at).getFullYear()
  const b = new Date(c.last_at || c.first_at).getFullYear()
  return a === b ? String(a) : `${a}–${b}`
}

/** The first letter of a name, for avatars. */
export function initial(name: string): string {
  const m = name.trim().match(/\p{L}|\p{N}/u)
  return (m?.[0] ?? '?').toUpperCase()
}

/** A stable colour for a name, one of a few soft tones. */
export function tone(name: string): number {
  let h = 0
  for (const ch of name) h = (h * 31 + ch.codePointAt(0)!) >>> 0
  return h % 4
}

export function fileUrl(id: string, variant: 'thumb' | 'preview' | 'poster' | 'original' | 'download'): string {
  return `/api/message-files/${encodeURIComponent(id)}/${variant}`
}

/** A short label for a file type: PDF, DOCX, XLSX. */
export function fileType(f: MessageFile): string {
  const ext = f.name?.match(/\.([a-z0-9]{1,5})$/i)?.[1]
  if (ext) return ext.toUpperCase()
  if (f.mime === 'application/pdf') return 'PDF'
  return t('messages.attachment')
}

/** The colour family of a file type badge. */
export function fileFamily(f: MessageFile): 'pdf' | 'sheet' | 'text' | 'slides' | 'other' {
  const type = fileType(f).toLowerCase()
  if (type === 'pdf') return 'pdf'
  if (['xls', 'xlsx', 'ods', 'csv', 'numbers'].includes(type)) return 'sheet'
  if (['doc', 'docx', 'odt', 'rtf', 'txt', 'pages'].includes(type)) return 'text'
  if (['ppt', 'pptx', 'odp', 'key'].includes(type)) return 'slides'
  return 'other'
}

/** Splits a search snippet into plain and matching parts. */
export function snippetParts(s: string): { s: string; hit: boolean }[] {
  const out: { s: string; hit: boolean }[] = []
  for (const piece of s.split('\u0002')) {
    const [hit, rest] = piece.includes('\u0003') ? piece.split('\u0003') : [undefined, piece]
    if (hit) out.push({ s: hit, hit: true })
    if (rest) out.push({ s: rest, hit: false })
  }
  return out
}

/** Midnight of a YYYY-MM-DD date in the browser's time zone, in milliseconds. */
export function startOfDay(date: string): number {
  const [y, m, d] = date.split('-').map(Number)
  return new Date(y, m - 1, d).getTime()
}

/** A YYYY-MM-DD date in the browser's time zone. */
export function isoDate(ms: number): string {
  const d = new Date(ms)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}
