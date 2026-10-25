# Translating

The app is available in English, Dutch, French, German and Spanish. English is the source language.

## Where the text lives

All interface text is in `web/src/lib/i18n/`, one JSON file per language:

```txt
web/src/lib/i18n/
├── en.json
├── nl.json
├── fr.json
├── de.json
└── es.json
```

Keys are grouped by screen. Never put interface text directly in code.

## Plurals

Text that depends on a number has a variant per plural form, using the categories of `Intl.PluralRules`:

```json
"memories_one": "{count} memory",
"memories_other": "{count} memories"
```

Languages with more plural forms can add `_few` or `_many`; missing forms fall back to `_other`.

## Placeholders

Words in braces, such as `{count}` or `{platform}`, are filled in by the app. Keep them exactly as they are. You can move them within the sentence.

## Export guides

The step-by-step instructions for requesting exports are in the same files, under `guides`. Use the menu names the platform itself shows in that language. When a platform changes its menus, updating the guide is a very welcome contribution.

## Style

- State what something is or does. Avoid filler and reassurance.
- Use the informal form where the language allows it (*je*, *du*, *tú*, but *vous* in French).
- Keep the wording short enough for a phone screen.

## Checking

```sh
cd web && npm test
```

The test fails when a language is missing a key or a placeholder.
