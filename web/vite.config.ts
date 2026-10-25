import { svelte } from '@sveltejs/vite-plugin-svelte'
import { writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { defineConfig, type Plugin } from 'vite'

const outDir = fileURLToPath(new URL('../internal/web/dist', import.meta.url))

// The Go server embeds the build output. Keep a placeholder file in the folder
// so the embed directive still compiles after the folder is emptied.
function keepPlaceholder(): Plugin {
  return {
    name: 'afterfeed-keep-placeholder',
    closeBundle() {
      writeFileSync(`${outDir}/.gitkeep`, '')
    },
  }
}

export default defineConfig({
  plugins: [svelte(), keepPlaceholder()],
  build: {
    outDir,
    emptyOutDir: true,
    target: 'es2022',
    chunkSizeWarningLimit: 1200,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': 'http://localhost:8080',
    },
  },
})
