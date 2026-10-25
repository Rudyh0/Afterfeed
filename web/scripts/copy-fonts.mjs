// Copies the Geist font files into public/fonts so they are served from a
// stable path. index.html preloads them for the splash and the app.
import { copyFileSync, mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const src = join(root, 'node_modules/@fontsource-variable/geist')
const dest = join(root, 'public/fonts')
mkdirSync(dest, { recursive: true })
for (const file of ['geist-latin-wght-normal.woff2', 'geist-latin-ext-wght-normal.woff2']) {
  copyFileSync(join(src, 'files', file), join(dest, file))
}
copyFileSync(join(src, 'LICENSE'), join(dest, 'GEIST-LICENSE.txt'))
