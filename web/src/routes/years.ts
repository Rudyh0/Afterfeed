import { t } from '../lib/i18n/index.svelte'

export interface YearSummary {
  year: number
  memories: number
  photos: number
  videos: number
  covers?: { id: string; kind: string }[]
}

/** "214 memories · 1,032 photos · 41 videos", leaving out zeros. */
export function countsLine(c: { memories?: number; photos: number; videos: number }): string {
  const parts: string[] = []
  if (c.memories !== undefined) parts.push(t('count.memories', { count: c.memories }))
  if (c.photos) parts.push(t('count.photos', { count: c.photos }))
  if (c.videos) parts.push(t('count.videos', { count: c.videos }))
  return parts.join(' · ')
}
