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

export const library = $state<{ sources: Source[]; loaded: boolean; importing: RunningImport | null }>({
  sources: [],
  loaded: false,
  importing: null,
})

let timer: ReturnType<typeof setTimeout> | undefined

export async function loadLibrary() {
  clearTimeout(timer)
  try {
    const res = await get<{ sources: Source[]; importing?: RunningImport }>('/library')
    library.sources = res.sources
    library.importing = res.importing ?? null
  } catch {
    // The sidebar simply shows no sources.
  } finally {
    library.loaded = true
  }
  // While an import runs, follow it; the sources change when it is done.
  if (library.importing) timer = setTimeout(loadLibrary, 5000)
}
