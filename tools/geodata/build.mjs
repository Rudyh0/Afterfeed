// Writes the place names Afterfeed uses for offline reverse geocoding: every
// populated place with at least 1,000 inhabitants, from GeoNames
// (https://www.geonames.org, CC BY 4.0) as packaged by all-the-cities.
//
// Format: one place per line, tab separated: latitude, longitude (both times
// 100,000, as integers), country code, population, name. Gzip compressed.
import { createRequire } from 'node:module'
import { writeFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'

const require = createRequire(import.meta.url)
const cities = require('all-the-cities')

// Sections of places, and places that no longer exist, are left out.
const skip = new Set(['PPLX', 'PPLH', 'PPLQ', 'PPLW', 'PPLCH'])
const rows = cities
  .filter((c) => !skip.has(c.featureCode) && c.country && c.name)
  .map((c) => {
    const [lon, lat] = c.loc.coordinates
    return [Math.round(lat * 1e5), Math.round(lon * 1e5), c.country, c.population, c.name.replace(/[\t\n]/g, ' ')]
  })
  .sort((a, b) => a[2].localeCompare(b[2]) || a[0] - b[0] || a[1] - b[1])

const tsv = rows.map((r) => r.join('\t')).join('\n') + '\n'
const out = process.argv[2] ?? 'cities.tsv.gz'
writeFileSync(out, gzipSync(Buffer.from(tsv), { level: 9 }))
console.log(`${rows.length} places, ${(tsv.length / 1e6).toFixed(1)} MB, written to ${out}`)
