import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const locales = ['en', 'nl', 'fr', 'de', 'es']

function flatten(obj: Record<string, unknown>, prefix = '', out: Record<string, string> = {}) {
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object') flatten(v as Record<string, unknown>, key, out)
    else out[key] = String(v)
  }
  return out
}

function load(l: string) {
  return flatten(JSON.parse(readFileSync(new URL(`../src/lib/i18n/${l}.json`, import.meta.url), 'utf8')))
}

const en = load('en')

for (const l of locales.filter((l) => l !== 'en')) {
  test(`${l} has exactly the same keys as en`, () => {
    const d = load(l)
    const missing = Object.keys(en).filter((k) => !(k in d))
    const extra = Object.keys(d).filter((k) => !(k in en))
    assert.deepEqual(missing, [], `missing in ${l}`)
    assert.deepEqual(extra, [], `not in en`)
  })

  test(`${l} keeps every placeholder`, () => {
    const d = load(l)
    for (const [k, v] of Object.entries(en)) {
      const want: string[] = [...(v.match(/\{\w+\}/g) ?? [])].sort()
      const got: string[] = [...(d[k]?.match(/\{\w+\}/g) ?? [])].sort()
      // Singular forms may write the number out, so only check that no new
      // placeholders appear there.
      if (k.endsWith('_one')) {
        assert.ok(got.every((p) => want.includes(p)), `${l}:${k} has unknown placeholders`)
      } else {
        assert.deepEqual(got, want, `${l}:${k}`)
      }
    }
  })
}
