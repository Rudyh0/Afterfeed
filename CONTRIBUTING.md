# Contributing to Afterfeed

Thanks for your interest in Afterfeed. This document explains what kind of contributions are welcome and how to get them merged.

## What is welcome

- **Bug fixes** – open a pull request directly. Please describe the bug and how to reproduce it.
- **Translations** – improvements to existing languages and corrections to the export guides.
- **Documentation** – fixes, clarifications and installation notes for platforms we have not covered.
- **Importer fixes** – platforms change their export formats without notice. If an import breaks, a fix with an anonymised sample of the changed structure is very valuable.

## New features

Please **open an issue before you start working on a new feature**. Afterfeed has a deliberately small scope: a private, calm archive of your own memories. A feature that makes sense in another app can still be a poor fit here. Discussing it first avoids work that cannot be merged.

Things that are out of scope:

- Sharing, public profiles, comments, likes or anything that turns Afterfeed into a social network
- Notifications, streaks or other mechanics designed to bring you back
- Hosted or multi-tenant versions

## How pull requests are handled

- All changes go through pull requests. Nobody pushes to `main` directly.
- Every pull request is reviewed and approved by the maintainer before it is merged.
- The automated checks (tests, linting and builds) must pass.
- Keep pull requests focused. One fix or one feature per pull request.

## Development setup

You need:

- Go 1.24 or newer, with a C compiler (the SQLite driver uses cgo)
- Node.js 22 or newer
- `ffmpeg`, `vipsthumbnail` (libvips) and `exiftool` for media processing

Then:

```sh
make dev      # runs the backend and the frontend dev server
make test     # runs all tests
make check    # runs linters and type checks
make build    # builds the frontend and the backend binary
```

The backend serves the API on port 8080. The frontend dev server proxies API calls to it.

### Test data

Never commit real exports, not even your own. The `.gitignore` excludes zip files and the `dev-data/` folder. Importer tests build their fixtures from small, fake JSON files in `internal/importer/*/testdata/`. If you need to reproduce a problem with a real export, anonymise the relevant JSON structure and replace all text, names and media with placeholders.

## Code style

- Go: `gofmt` and `go vet`
- Frontend: Prettier and `svelte-check`
- User-facing text never lives in code. Add it to `web/src/lib/i18n/en.json` and, if you can, to the other languages.
- Interface text states what something is or does. Avoid reassuring or explanatory filler.

## Commit messages

Use the imperative mood and keep the first line under 72 characters, for example `Fix duplicate photos after re-import`.

## License

By contributing, you agree that your contributions are licensed under the [AGPL-3.0](LICENSE).
