# Development

## Requirements

- Go 1.24 or newer with a C compiler (the SQLite driver uses cgo)
- Node.js 22 or newer
- `ffmpeg`, `vipsthumbnail` (libvips) and `exiftool`

On Debian or Ubuntu:

```sh
sudo apt install build-essential ffmpeg libvips-tools libimage-exiftool-perl
```

On macOS with Homebrew:

```sh
brew install go node ffmpeg vips exiftool
```

## Running locally

```sh
make dev
```

This runs the backend on port 8080 and the frontend dev server on port 5173. Open `http://localhost:5173`. Local data goes into `dev-data/`, which is ignored by git.

## Project structure

```txt
cmd/afterfeed/        Server and command-line entry point
internal/
  auth/               Accounts, sessions, two-factor authentication, vault PIN
  config/             Environment variables and paths
  importer/           Import framework and one package per platform
  library/            Memories, messages, search, map and metadata edits
  media/              Thumbnails, previews and metadata in downloads
  server/             HTTP API and serving the web app
  store/              SQLite database and migrations
  web/                The built web app, embedded in the binary
web/                  Svelte web app
docs/                 This documentation
```

## Checks

```sh
make test     # Go and web tests
make check    # gofmt, go vet and svelte-check
```

The same checks run on every pull request.

## Building the image

```sh
docker build -t afterfeed:dev .
```
