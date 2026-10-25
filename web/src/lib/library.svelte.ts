// Library-wide information shared by several screens, such as the sources in
// the archive. Refreshed after imports.

import { get } from './api'

export interface Source {
  platform: string
  first_year: number
  last_year: number
  memories: number
}

export const library = $state<{ sources: Source[]; loaded: boolean }>({ sources: [], loaded: false })

export async function loadLibrary() {
  try {
    const res = await get<{ sources: Source[] }>('/library')
    library.sources = res.sources
  } catch {
    // The sidebar simply shows no sources.
  } finally {
    library.loaded = true
  }
}
