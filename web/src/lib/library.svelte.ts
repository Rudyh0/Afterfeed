// Library-wide information shared by several screens, such as the sources in
// the archive and a running import. Refreshed after imports.

import { get } from './api'

export interface Source {
  platform: string
  first_year: number
  last_year: number
  memories: number
  conversations?: number
}

export interface RunningImport {
  id: number
  platform: string
  phase: string
  progress: number
}

export interface PreviewProgress {
  left: number
  total: number
}

export const library = $state<{
  sources: Source[]
  loaded: boolean
  importing: RunningImport | null
  previews: PreviewProgress | null
  /** Text in photos being read, for search. */
  readingText: PreviewProgress | null
}>({
  sources: [],
  loaded: false,
  importing: null,
  previews: null,
  readingText: null,
})

/** Background work to show in the navigation, after a running import. */
export function backgroundWork(): { label: string; left: number; total: number } | null {
  if (library.previews) return { label: 'nav.previews', ...library.previews }
  if (library.readingText) return { label: 'nav.reading_text', ...library.readingText }
  return null
}

let timer: ReturnType<typeof setTimeout> | undefined
// Previews start a moment after an import ends, so look once more then.
let followUp = false

export async function loadLibrary() {
  clearTimeout(timer)
  try {
    const res = await get<{
      sources: Source[]
      importing?: RunningImport
      previews?: PreviewProgress
      reading_text?: PreviewProgress
    }>('/library')
    library.sources = res.sources
    library.importing = res.importing ?? null
    library.previews = res.previews ?? null
    library.readingText = res.reading_text ?? null
  } catch {
    // The sidebar simply shows no sources.
  } finally {
    library.loaded = true
  }
  // While an import runs or previews are made, follow it; the sources change
  // when an import is done.
  if (library.importing || library.previews || library.readingText) {
    followUp = !!library.importing || !!library.previews
    timer = setTimeout(loadLibrary, 5000)
  } else if (followUp) {
    followUp = false
    timer = setTimeout(loadLibrary, 8000)
  }
}
